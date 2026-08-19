package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/middleware"
)

// ─── Connector Manager (PG-persistent) ───────────────────────
// Manages connector lifecycle with PostgreSQL as source of truth.
// Maintains an in-memory cache for fast access.

type Manager struct {
	mu         sync.RWMutex
	pgPool     *pgxpool.Pool
	vault      VaultHandle
	connectors map[string]Connector
	v2conns    map[string]ConnectorV2
	configs    map[string]ConnectorConfig
	results    map[string]*SyncResult
	health     map[string]*HealthReport
}

func NewManager(pgPool *pgxpool.Pool) *Manager {
	return &Manager{
		pgPool:     pgPool,
		connectors: make(map[string]Connector),
		v2conns:    make(map[string]ConnectorV2),
		configs:    make(map[string]ConnectorConfig),
		results:    make(map[string]*SyncResult),
		health:     make(map[string]*HealthReport),
	}
}

// WithVault attaches a credential vault. When set, RegisterSecure() can
// move plaintext secrets (Password/ClientSecret/Cert) into AES-256-GCM
// encrypted storage and replace them with a vault_secret_id reference.
// The vault is optional — absence just disables the secure registration path.
func (m *Manager) WithVault(v VaultHandle) *Manager {
	m.vault = v
	return m
}

// VaultHandle is the minimal contract RegisterSecure needs from the vault.
// It mirrors *vault.Vault's Store/Retrieve methods so manager.go doesn't
// need to import the vault package (avoids an import cycle in tests).
type VaultHandle interface {
	Store(ctx context.Context, name, secretType, reference, plaintext string) (string, error)
	Retrieve(ctx context.Context, id string) (string, error)
}

// asV2 returns the ConnectorV2 for a connector ID, caching the
// V2 view so subsequent calls are a map lookup.
// - If the connector natively implements ConnectorV2, it's returned directly.
// - Otherwise, it's wrapped by the V1ToV2Adapter (no code changes needed).
func (m *Manager) asV2(id string) ConnectorV2 {
	m.mu.RLock()
	if v2, ok := m.v2conns[id]; ok {
		m.mu.RUnlock()
		return v2
	}
	conn, ok := m.connectors[id]
	cfg, hasCfg := m.configs[id]
	m.mu.RUnlock()

	if !ok || !hasCfg {
		return nil
	}

	// Check for native V2
	if v2, ok := conn.(ConnectorV2); ok {
		m.mu.Lock()
		m.v2conns[id] = v2
		m.mu.Unlock()
		return v2
	}

	// Wrap V1 → V2 via adapter
	v2 := AdaptConnectorV1(conn, cfg)
	m.mu.Lock()
	m.v2conns[id] = v2
	m.mu.Unlock()
	return v2
}

// LoadAll loads all persisted connectors from PostgreSQL into memory.
// Call this once on startup.
func (m *Manager) LoadAll(ctx context.Context) ([]ConnectorConfig, error) {
	// Use a transaction to set tenant context for RLS bypass
	tx, err := m.pgPool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("manager: load connectors begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Set tenant context to default tenant to bypass RLS policies
	// This allows loading all connectors regardless of tenant isolation
	const defaultTenant = "00000000-0000-0000-0000-000000000001"
	if _, err := tx.Exec(ctx, "SET LOCAL app.current_tenant = '"+defaultTenant+"'"); err != nil {
		return nil, fmt.Errorf("manager: set tenant context: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, tenant_id, name, connector_type, status, config, last_sync_at, last_error,
		       COALESCE(schedule_cron, ''), COALESCE(owner_identity_id::text, ''),
		       COALESCE(risk_weight, 0), COALESCE(connector_governance_status, 'active'),
		       COALESCE(vault_secret_id::text, ''), COALESCE(last_sync_duration_ms, 0)
		FROM connectors ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("manager: load connectors: %w", err)
	}
	defer rows.Close()

	var loaded []ConnectorConfig
	for rows.Next() {
		var id, tenantID, name, ctype, status string
		var configJSON []byte
		var lastSync *time.Time
		var lastError *string
		var scheduleCron, ownerID, governanceStatus, vaultSecretID string
		var riskWeight, lastSyncDurMS int

		if err := rows.Scan(&id, &tenantID, &name, &ctype, &status, &configJSON, &lastSync, &lastError,
			&scheduleCron, &ownerID, &riskWeight, &governanceStatus, &vaultSecretID, &lastSyncDurMS); err != nil {
			log.Printf("[CONNECTOR] scan error: %v", err)
			continue
		}

		var cfg ConnectorConfig
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			cfg = ConnectorConfig{
				ID: id, TenantID: tenantID, Name: name,
				Type: ConnectorType(ctype), Status: ConnectorStatus(status),
			}
		}
		cfg.ID = id
		cfg.TenantID = tenantID
		cfg.Name = name
		cfg.Type = ConnectorType(ctype)
		cfg.Status = ConnectorStatus(status)
		if lastSync != nil {
			cfg.LastSyncAt = lastSync
		}
		if lastError != nil {
			cfg.LastError = *lastError
		}
		// New platform fields
		cfg.ScheduleCron = scheduleCron
		cfg.OwnerIdentityID = ownerID
		cfg.RiskWeight = riskWeight
		cfg.ConnectorGovernanceStatus = governanceStatus
		cfg.VaultSecretID = vaultSecretID
		cfg.LastSyncDurationMS = lastSyncDurMS

		// Create the connector instance
		conn, err := m.newConnector(cfg.Type)
		if err != nil {
			log.Printf("[CONNECTOR] skip %s: %v", cfg.Name, err)
			continue
		}
		if err := conn.Configure(cfg); err != nil {
			log.Printf("[CONNECTOR] configure %s: %v", cfg.Name, err)
			continue
		}

		m.mu.Lock()
		m.connectors[id] = conn
		m.configs[id] = cfg
		m.health[id] = &HealthReport{
			ConnectorID:       id,
			ConnectorName:     name,
			Status:            string(cfg.Status),
			LastSyncAt:        cfg.LastSyncAt,
			LastError:         cfg.LastError,
			DeltaSupported:    m.supportsDelta(conn),
			SupportsSchema:    m.supportsSchema(conn),
		}
		m.mu.Unlock()

		loaded = append(loaded, cfg)
		log.Printf("[CONNECTOR] Loaded: %s (%s) [%s]", name, ctype, status)
	}

	log.Printf("[CONNECTOR] Loaded %d connectors from database", len(loaded))
	return loaded, nil
}

func (m *Manager) supportsDelta(conn Connector) bool {
	switch conn.Type() {
	case ConnectorTypeEntraID:
		return true // Microsoft Graph supports delta queries
	default:
		return false
	}
}

func (m *Manager) supportsSchema(conn Connector) bool {
	// All connectors can introspect schema
	return true
}

// ─── Registration ────────────────────────────────────────────

func (m *Manager) Register(ctx context.Context, config ConnectorConfig) (string, error) {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}
	if config.Status == "" {
		config.Status = ConnectorStatusDisconnected
	}
	if config.SyncIntervalMinutes <= 0 {
		config.SyncIntervalMinutes = 60
	}
	if config.ScheduleCron == "" {
		config.ScheduleCron = "*/20 * * * *"
	}
	if config.RiskWeight == 0 {
		config.RiskWeight = 50
	}
	if config.ConnectorGovernanceStatus == "" {
		config.ConnectorGovernanceStatus = "active"
	}
	if config.TenantID == "" {
		config.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	conn, err := m.newConnector(config.Type)
	if err != nil {
		return "", fmt.Errorf("manager: create connector: %w", err)
	}
	if err := conn.Configure(config); err != nil {
		return "", fmt.Errorf("manager: configure connector: %w", err)
	}

	// Persist to PostgreSQL — write config JSONB plus the new platform columns.
	// Use TenantDB to get the transaction from context (set by TenantMiddleware)
	// so FK constraints can see rows inserted in the same transaction.
	db := middleware.TenantDB(ctx, m.pgPool)
	cfgJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("manager: marshal config: %w", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO connectors (id, tenant_id, name, connector_type, status, config,
		                        schedule_cron, owner_identity_id, risk_weight,
		                        connector_governance_status, vault_secret_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, '')::uuid)
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			connector_type             = EXCLUDED.connector_type,
			config                     = EXCLUDED.config,
			status                     = EXCLUDED.status,
			schedule_cron              = EXCLUDED.schedule_cron,
			owner_identity_id          = EXCLUDED.owner_identity_id,
			risk_weight                = EXCLUDED.risk_weight,
			connector_governance_status = EXCLUDED.connector_governance_status,
			vault_secret_id            = EXCLUDED.vault_secret_id,
			updated_at                 = NOW()
	`, config.ID, config.TenantID, config.Name, string(config.Type), string(config.Status), cfgJSON,
		config.ScheduleCron, nullableUUID(config.OwnerIdentityID), config.RiskWeight,
		config.ConnectorGovernanceStatus, config.VaultSecretID)
	if err != nil {
		return "", fmt.Errorf("manager: persist connector: %w", err)
	}

	// Cache in memory
	m.mu.Lock()
	m.connectors[config.ID] = conn
	m.configs[config.ID] = config
	m.health[config.ID] = &HealthReport{
		ConnectorID:    config.ID,
		ConnectorName:  config.Name,
		Status:         string(config.Status),
		DeltaSupported: m.supportsDelta(conn),
		SupportsSchema: m.supportsSchema(conn),
	}
	m.mu.Unlock()

	log.Printf("[CONNECTOR] Registered: %s (%s) as %s", config.Name, config.Type, config.ID)
	return config.ID, nil
}

// RegisterSecure is the production path: it takes plaintext credentials from
// the supplied config, vaults them via the attached SecretStore, and persists
// only the ciphertext handle (vault_secret_id) — the plaintext never lands
// in the connectors.config JSONB column. Caller passes the request context
// so the vault can read the tenant_id for RLS-scoped storage.
//
// The vault is required (WithVault). If no vault is attached, RegisterSecure
// falls back to the standard Register path with a warning log.
func (m *Manager) RegisterSecure(ctx context.Context, config ConnectorConfig) (string, error) {
	if m.vault == nil {
		log.Printf("[CONNECTOR] RegisterSecure called without a vault — falling back to plaintext Register (not for production)")
		return m.Register(ctx, config)
	}

	// Collect the cleartext secret value to vault. Preference order:
	// ClientSecret (OAuth client_credentials, most common in cloud apps),
	// Password (HTTP basic / LDAP bind), Cert (mTLS), API key in Properties.
	plaintext := config.ClientSecret
	secretType := "client_secret"
	if plaintext == "" && config.Password != "" {
		plaintext = config.Password
		secretType = "connector_password"
	}
	if plaintext == "" && config.Cert != "" {
		plaintext = config.Cert
		secretType = "tls_cert"
	}

	if plaintext == "" {
		// No credentials to vault — register normally with vault_secret_id=NULL.
		return m.Register(ctx, config)
	}

	// Vault the secret. Always generate a fresh UUID as the reference
	// so the vault store receives a valid UUID format regardless of what
	// ID value was supplied in the connector config.
	connectorID := uuid.New().String()
	secretName := fmt.Sprintf("connector-%s-%s", config.Name, secretType)
	vaultID, err := m.vault.Store(ctx, secretName, secretType, connectorID, plaintext)
	if err != nil {
		return "", fmt.Errorf("manager: RegisterSecure: vault secret: %w", err)
	}

	// Replace plaintext + record the vault handle, then register normally.
	// Storing cleared fields + VaultSecretID in config JSONB means any
	// caller that loads the config later sees only the vault reference.
	config.VaultSecretID = vaultID
	config.ClientSecret = ""
	config.Password = ""
	config.Cert = ""
	log.Printf("[CONNECTOR] RegisterSecure: vaulted %s for connector %s (vault_id=%s)", secretType, config.Name, vaultID)
	return m.Register(ctx, config)
}

// nullableUUID returns a *string suitable for pgx pgxpool.Exec: empty → nil
// so Postgres writes NULL instead of failing on ''::uuid.
func nullableUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (m *Manager) Unregister(ctx context.Context, id string) error {
	m.mu.Lock()
	if conn, ok := m.connectors[id]; ok {
		conn.Disconnect(ctx)
	}
	delete(m.connectors, id)
	delete(m.v2conns, id)
	delete(m.configs, id)
	delete(m.results, id)
	delete(m.health, id)
	m.mu.Unlock()

	// Delete from PostgreSQL
	if _, err := m.pgPool.Exec(ctx, `DELETE FROM connectors WHERE id = $1`, id); err != nil {
		return fmt.Errorf("manager: delete connector: %w", err)
	}
	// Also clean up connector_identities
	m.pgPool.Exec(ctx, `DELETE FROM connector_identities WHERE connector_id = $1`, id)
	return nil
}

// ─── Connection Management ───────────────────────────────────

func (m *Manager) Connect(ctx context.Context, id string) error {
	m.mu.RLock()
	conn, ok := m.connectors[id]
	config, hasCfg := m.configs[id]
	m.mu.RUnlock()

	if !ok || !hasCfg {
		return fmt.Errorf("manager: connector not found: %s", id)
	}

	if err := conn.Connect(ctx); err != nil {
		config.Status = ConnectorStatusError
		config.LastError = err.Error()
		m.updateConfig(ctx, config)
		m.updateHealth(id, ConnectorStatusError, err.Error())
		return err
	}

	config.Status = ConnectorStatusConnected
	config.LastError = ""
	m.updateConfig(ctx, config)
	m.updateHealth(id, ConnectorStatusConnected, "")
	return nil
}

func (m *Manager) Disconnect(ctx context.Context, id string) error {
	m.mu.RLock()
	conn, ok := m.connectors[id]
	config, hasCfg := m.configs[id]
	m.mu.RUnlock()

	if !ok || !hasCfg {
		return fmt.Errorf("manager: connector not found: %s", id)
	}

	conn.Disconnect(ctx)
	config.Status = ConnectorStatusDisconnected
	m.updateConfig(ctx, config)
	m.updateHealth(id, ConnectorStatusDisconnected, "")
	return nil
}

// ─── Queries ──────────────────────────────────────────────────

func (m *Manager) GetConnector(id string) (Connector, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connectors[id]
	if !ok {
		return nil, fmt.Errorf("manager: connector not found: %s", id)
	}
	return conn, nil
}

func (m *Manager) GetConfig(id string) (ConnectorConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	config, ok := m.configs[id]
	if !ok {
		return ConnectorConfig{}, fmt.Errorf("manager: connector not found: %s", id)
	}
	return config, nil
}

func (m *Manager) List() []ConnectorConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	configs := make([]ConnectorConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		configs = append(configs, cfg)
	}
	return configs
}

// ─── Sync ─────────────────────────────────────────────────────

func (m *Manager) SyncUsers(ctx context.Context, id string) (*SyncResult, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	config := m.configs[id]
	isConnected := config.Status == ConnectorStatusConnected
	m.mu.RUnlock()

	result := &SyncResult{
		ConnectorID:   id,
		ConnectorName: config.Name,
		ConnectorType: string(config.Type),
		StartedAt:     time.Now(),
	}

	if !isConnected {
		if err := conn.Connect(ctx); err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Success = false
			result.CompletedAt = time.Now()
			config.Status = ConnectorStatusError
			config.LastError = err.Error()
			m.updateConfig(ctx, config)
			m.updateHealth(id, ConnectorStatusError, err.Error())
			m.results[id] = result
			return result, err
		}
	}

	config.Status = ConnectorStatusSyncing
	m.updateConfig(ctx, config)

	start := time.Now()
	remoteUsers, err := conn.ListUsers(ctx)
	elapsed := time.Since(start)

	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("list users: %v", err))
		result.Success = false
		result.CompletedAt = time.Now()
		config.Status = ConnectorStatusError
		config.LastError = err.Error()
		m.updateConfig(ctx, config)
		m.updateHealthWithDuration(id, ConnectorStatusError, err.Error(), elapsed)
		m.results[id] = result
		return result, err
	}

	now := time.Now()
	result.UsersTotal = len(remoteUsers)
	result.Users = remoteUsers
	config.LastSyncAt = &now
	config.Status = ConnectorStatusConnected
	config.LastError = ""
	result.Success = true
	result.CompletedAt = now

	m.updateConfig(ctx, config)
	m.updateHealthWithDuration(id, ConnectorStatusConnected, "", elapsed)
	m.mu.Lock()
	if h := m.health[id]; h != nil {
		h.TotalUsersSynced = len(remoteUsers)
	}
	m.mu.Unlock()
	m.results[id] = result

	log.Printf("[CONNECTOR] Sync complete for %s: %d users in %s", config.Name, len(remoteUsers), elapsed.Round(time.Millisecond))
	return result, nil
}

// SyncUsersDelta performs an incremental sync using the connector's delta token.
// Falls back to full sync if delta is not supported.
func (m *Manager) SyncUsersDelta(ctx context.Context, id string) (*SyncResult, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	config := m.configs[id]
	deltaToken := config.DeltaToken
	m.mu.RUnlock()

	result := &SyncResult{
		ConnectorID:   id,
		ConnectorName: config.Name,
		ConnectorType: string(config.Type),
		StartedAt:     time.Now(),
	}

	// Try delta first, fall back to full sync
	start := time.Now()
	users, newToken, err := conn.ListUsersDelta(ctx, deltaToken)
	elapsed := time.Since(start)

	if err == ErrDeltaNotSupported {
		log.Printf("[CONNECTOR] %s: delta not supported, falling back to full sync", config.Name)
		return m.SyncUsers(ctx, id)
	}

	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("delta sync: %v", err))
		result.Success = false
		result.CompletedAt = time.Now()
		config.Status = ConnectorStatusError
		config.LastError = err.Error()
		m.updateConfig(ctx, config)
		m.updateHealthWithDuration(id, ConnectorStatusError, err.Error(), elapsed)
		m.results[id] = result
		return result, err
	}

	// Save new delta token
	if newToken != "" {
		m.mu.Lock()
		config.DeltaToken = newToken
		m.configs[id] = config
		m.mu.Unlock()
		m.updateConfig(ctx, config)
	}

	now := time.Now()
	result.UsersTotal = len(users)
	result.Users = users
	result.DeltaToken = newToken
	config.LastSyncAt = &now
	config.Status = ConnectorStatusConnected
	config.LastError = ""
	result.Success = true
	result.CompletedAt = now

	m.updateConfig(ctx, config)
	m.updateHealthWithDuration(id, ConnectorStatusConnected, "", elapsed)
	m.results[id] = result

	log.Printf("[CONNECTOR] Delta sync complete for %s: %d changes in %s", config.Name, len(users), elapsed.Round(time.Millisecond))
	return result, nil
}

// ─── Group Sync ───────────────────────────────────────────────

func (m *Manager) SyncGroups(ctx context.Context, id string) ([]ConnectorGroup, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	config := m.configs[id]
	m.mu.RUnlock()

	start := time.Now()
	groups, err := conn.ListGroups(ctx)
	elapsed := time.Since(start)

	if err != nil {
		config.LastError = fmt.Sprintf("group sync: %v", err)
		config.Status = ConnectorStatusError
		m.updateConfig(ctx, config)
		m.updateHealthWithDuration(id, ConnectorStatusError, err.Error(), elapsed)
		return nil, err
	}

	config.Status = ConnectorStatusConnected
	config.LastError = ""
	m.updateConfig(ctx, config)
	m.updateHealthWithDuration(id, ConnectorStatusConnected, "", elapsed)

	log.Printf("[CONNECTOR] Groups sync for %s: %d groups in %s", config.Name, len(groups), elapsed.Round(time.Millisecond))
	return groups, nil
}

// ─── Entitlement Sync ────────────────────────────────────

func (m *Manager) SyncEntitlements(ctx context.Context, id string) ([]ConnectorEntitlement, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	config := m.configs[id]
	m.mu.RUnlock()

	start := time.Now()
	entitlements, err := conn.ListEntitlements(ctx)
	elapsed := time.Since(start)

	if err == ErrNotSupported {
		log.Printf("[CONNECTOR] %s: entitlements not supported", config.Name)
		return nil, nil
	}
	if err != nil {
		config.LastError = fmt.Sprintf("entitlement sync: %v", err)
		config.Status = ConnectorStatusError
		m.updateConfig(ctx, config)
		m.updateHealthWithDuration(id, ConnectorStatusError, err.Error(), elapsed)
		return nil, err
	}

	config.Status = ConnectorStatusConnected
	config.LastError = ""
	m.updateConfig(ctx, config)
	m.updateHealthWithDuration(id, ConnectorStatusConnected, "", elapsed)

	log.Printf("[CONNECTOR] Entitlement sync for %s: %d entitlements in %s", config.Name, len(entitlements), elapsed.Round(time.Millisecond))
	return entitlements, nil
}

// ─── Resource Sync ───────────────────────────────────────

func (m *Manager) SyncResources(ctx context.Context, id string) ([]ConnectorResource, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	config := m.configs[id]
	m.mu.RUnlock()

	start := time.Now()
	resources, err := conn.ListResources(ctx)
	elapsed := time.Since(start)

	if err == ErrNotSupported {
		log.Printf("[CONNECTOR] %s: resources not supported", config.Name)
		return nil, nil
	}
	if err != nil {
		config.LastError = fmt.Sprintf("resource sync: %v", err)
		config.Status = ConnectorStatusError
		m.updateConfig(ctx, config)
		m.updateHealthWithDuration(id, ConnectorStatusError, err.Error(), elapsed)
		return nil, err
	}

	config.Status = ConnectorStatusConnected
	config.LastError = ""
	m.updateConfig(ctx, config)
	m.updateHealthWithDuration(id, ConnectorStatusConnected, "", elapsed)

	log.Printf("[CONNECTOR] Resource sync for %s: %d resources in %s", config.Name, len(resources), elapsed.Round(time.Millisecond))
	return resources, nil
}

// ─── Permission Catalog Sync ─────────────────────────────

func (m *Manager) SyncPermissions(ctx context.Context, id string) ([]ConnectorPermission, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	enumerator, ok := conn.(PermissionEnumerator)
	if !ok {
		log.Printf("[CONNECTOR] %s: permissions not supported", m.configs[id].Name)
		return nil, nil
	}

	m.mu.RLock()
	config := m.configs[id]
	m.mu.RUnlock()

	start := time.Now()
	permissions, err := enumerator.ListPermissions(ctx)
	elapsed := time.Since(start)

	if err == ErrNotSupported {
		log.Printf("[CONNECTOR] %s: permissions not supported", config.Name)
		return nil, nil
	}
	if err != nil {
		config.LastError = fmt.Sprintf("permission sync: %v", err)
		config.Status = ConnectorStatusError
		m.updateConfig(ctx, config)
		m.updateHealthWithDuration(id, ConnectorStatusError, err.Error(), elapsed)
		return nil, err
	}

	config.Status = ConnectorStatusConnected
	config.LastError = ""
	m.updateConfig(ctx, config)
	m.updateHealthWithDuration(id, ConnectorStatusConnected, "", elapsed)

	log.Printf("[CONNECTOR] Permission sync for %s: %d permissions in %s", config.Name, len(permissions), elapsed.Round(time.Millisecond))
	return permissions, nil
}

// FullSync performs a complete sync: users, groups, entitlements, and resources.
func (m *Manager) FullSync(ctx context.Context, id string) (*FullSyncResult, error) {
	result := &FullSyncResult{ConnectorID: id}

	// 1. Users
	usersResult, err := m.SyncUsers(ctx, id)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("users: %v", err))
	} else if usersResult != nil {
		result.Users = usersResult.Users
		result.UsersCreated = usersResult.UsersCreated
		result.UsersUpdated = usersResult.UsersUpdated
		result.UsersTotal = usersResult.UsersTotal
	}

	// 2. Groups
	groups, err := m.SyncGroups(ctx, id)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("groups: %v", err))
	} else {
		result.Groups = groups
	}

	// 3. Entitlements
	entitlements, err := m.SyncEntitlements(ctx, id)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("entitlements: %v", err))
	} else {
		result.Entitlements = entitlements
	}

	// 4. Resources
	resources, err := m.SyncResources(ctx, id)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("resources: %v", err))
	} else {
		result.Resources = resources
	}

	result.Success = len(result.Errors) == 0
	result.CompletedAt = time.Now()
	return result, nil
}

// ─── Schema Discovery ────────────────────────────────────────

func (m *Manager) GetConnectorSchema(ctx context.Context, id string) (*SchemaResult, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}
	return conn.DiscoverSchema(ctx)
}

// ─── Health ──────────────────────────────────────────────────

func (m *Manager) GetConnectorHealth(id string) (*HealthReport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.health[id]
	if !ok {
		return nil, fmt.Errorf("manager: connector not found: %s", id)
	}
	return h, nil
}

func (m *Manager) GetLastSyncResult(id string) *SyncResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.results[id]
}

func (m *Manager) GetConnectorUsers(ctx context.Context, id string) ([]ConnectorUser, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}
	return conn.ListUsers(ctx)
}

// ─── Internal Helpers ─────────────────────────────────────────

func (m *Manager) updateConfig(ctx context.Context, config ConnectorConfig) {
	config.UpdatedAt = time.Now()

	cfgJSON, err := json.Marshal(config)
	if err != nil {
		log.Printf("[MANAGER] updateConfig marshal: %v", err)
		return
	}
	if _, err := m.pgPool.Exec(ctx, `
		UPDATE connectors SET status = $1, config = $2, last_sync_at = $3, last_error = $4, updated_at = NOW()
		WHERE id = $5
	`, string(config.Status), cfgJSON, config.LastSyncAt, config.LastError, config.ID); err != nil {
		log.Printf("[MANAGER] updateConfig: %v", err)
		return
	}
	m.mu.Lock()
	m.configs[config.ID] = config
	m.mu.Unlock()
}

func (m *Manager) updateHealth(id string, status ConnectorStatus, lastError string) {
	m.updateHealthWithDuration(id, status, lastError, 0)
}

func (m *Manager) updateHealthWithDuration(id string, status ConnectorStatus, lastError string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.health[id]
	if !ok {
		h = &HealthReport{ConnectorID: id}
		m.health[id] = h
	}
	h.Status = string(status)
	h.LastError = lastError
	if lastError == "" {
		h.ConsecutiveSuccess++
		h.ConsecutiveErrors = 0
	} else {
		h.ConsecutiveErrors++
		h.ConsecutiveSuccess = 0
	}
	if duration > 0 {
		h.LastSyncDuration = duration.Round(time.Millisecond).String()
	}
}

// TestConnection tests a connector configuration without registering it.
func (m *Manager) TestConnection(ctx context.Context, config ConnectorConfig) error {
	conn, err := m.newConnector(config.Type)
	if err != nil {
		return err
	}
	if err := conn.Configure(config); err != nil {
		return err
	}
	return conn.TestConnection(ctx)
}

// newConnector creates a connector of the given type with the manager's vault.
func (m *Manager) newConnector(connType ConnectorType) (Connector, error) {
	switch connType {
	case ConnectorTypeEntraID:
		return NewEntraConnector(m.vault), nil
	case ConnectorTypeLDAP, ConnectorTypeAD:
		return NewLDAPConnector(), nil
	case ConnectorTypeSCIM, ConnectorTypeOkta, ConnectorTypeGeneric:
		return NewSCIMConnector(), nil
	case ConnectorTypeCSV:
		return NewCSVConnector(), nil
	default:
		return nil, fmt.Errorf("unknown connector type: %s", connType)
	}
}

// ─── Entity Resolution (Identity Stitching) ─────────────────────
// Before inserting a new Identity, check for existing identities with the same
// email or employee_id but different source. If found, create a Persona node
// in Neo4j and link both identities to it via a RESOLVES_TO relationship.

func (m *Manager) ResolveIdentity(ctx context.Context, tenantID, email, employeeID, source, newIdentityID string, neo4jDriver neo4j.DriverWithContext) (bool, error) {
	if email == "" && employeeID == "" {
		return false, nil
	}

	// Check for existing identities with same email or employee_id but different source
	var existingID, existingSource string
	query := `
		SELECT id, source::text FROM identities
		WHERE tenant_id = $1 AND (email = $2 OR employee_id = $3) AND id != $4
		LIMIT 1
	`
	args := []any{tenantID, email, employeeID, newIdentityID}
	if email == "" {
		query = `
			SELECT id, source::text FROM identities
			WHERE tenant_id = $1 AND employee_id = $2 AND id != $3
			LIMIT 1
		`
		args = []any{tenantID, employeeID, newIdentityID}
	}

	err := m.pgPool.QueryRow(ctx, query, args...).Scan(&existingID, &existingSource)
	if err != nil {
		// No existing identity found - this is a new unique identity
		return false, nil
	}

	// Found a match - create Persona node in Neo4j and link both identities
	if neo4jDriver != nil {
		session := neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		personaID := uuid.New().String()

		// Create or match Persona node and link both identities
		_, err := session.Run(ctx, `
			MERGE (p:Persona {id: $personaId})
			ON CREATE SET p.created_at = timestamp(), p.email = $email, p.employee_id = $employeeId
			WITH p
			MATCH (i1:Identity {uuid: $existingId})
			MATCH (i2:Identity {uuid: $newId})
			MERGE (i1)-[:RESOLVES_TO]->(p)
			MERGE (i2)-[:RESOLVES_TO]->(p)
		`, map[string]any{
			"personaId":  personaID,
			"email":      email,
			"employeeId": employeeID,
			"existingId": existingID,
			"newId":      newIdentityID,
		})
		if err != nil {
			log.Printf("[ENTITY_RES] neo4j stitch failed: %v", err)
		} else {
			log.Printf("[ENTITY_RES] stitched identity %s with %s via Persona %s", newIdentityID, existingID, personaID)
		}
	}

	// Return true to indicate this identity was stitched (caller should not create new PG row)
	return true, nil
}
