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
)

// ─── Connector Manager (PG-persistent) ───────────────────────
// Manages connector lifecycle with PostgreSQL as source of truth.
// Maintains an in-memory cache for fast access.

type Manager struct {
	mu         sync.RWMutex
	pgPool     *pgxpool.Pool
	connectors map[string]Connector
	configs    map[string]ConnectorConfig
	results    map[string]*SyncResult
	health     map[string]*HealthReport
}

func NewManager(pgPool *pgxpool.Pool) *Manager {
	return &Manager{
		pgPool:     pgPool,
		connectors: make(map[string]Connector),
		configs:    make(map[string]ConnectorConfig),
		results:    make(map[string]*SyncResult),
		health:     make(map[string]*HealthReport),
	}
}

// LoadAll loads all persisted connectors from PostgreSQL into memory.
// Call this once on startup.
func (m *Manager) LoadAll(ctx context.Context) ([]ConnectorConfig, error) {
	rows, err := m.pgPool.Query(ctx, `
		SELECT id, tenant_id, name, connector_type, status, config, last_sync_at, last_error
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

		if err := rows.Scan(&id, &tenantID, &name, &ctype, &status, &configJSON, &lastSync, &lastError); err != nil {
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

		// Create the connector instance
		conn, err := NewConnector(cfg.Type)
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
	if config.TenantID == "" {
		config.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	conn, err := NewConnector(config.Type)
	if err != nil {
		return "", fmt.Errorf("manager: create connector: %w", err)
	}
	if err := conn.Configure(config); err != nil {
		return "", fmt.Errorf("manager: configure connector: %w", err)
	}

	// Persist to PostgreSQL
	cfgJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("manager: marshal config: %w", err)
	}
	_, err = m.pgPool.Exec(ctx, `
		INSERT INTO connectors (id, tenant_id, name, connector_type, status, config)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, name) DO UPDATE SET
			connector_type = EXCLUDED.connector_type,
			config         = EXCLUDED.config,
			status         = EXCLUDED.status,
			updated_at     = NOW()
	`, config.ID, config.TenantID, config.Name, string(config.Type), string(config.Status), cfgJSON)
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

func (m *Manager) Unregister(ctx context.Context, id string) error {
	m.mu.Lock()
	if conn, ok := m.connectors[id]; ok {
		conn.Disconnect(ctx)
	}
	delete(m.connectors, id)
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
func TestConnection(ctx context.Context, config ConnectorConfig) error {
	conn, err := NewConnector(config.Type)
	if err != nil {
		return err
	}
	if err := conn.Configure(config); err != nil {
		return err
	}
	return conn.TestConnection(ctx)
}

// NewConnector creates a connector of the given type.
func NewConnector(connType ConnectorType) (Connector, error) {
	switch connType {
	case ConnectorTypeEntraID:
		return NewEntraConnector(), nil
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
