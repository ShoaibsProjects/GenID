package connector

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ─── Connector Manager ───────────────────────────────────────
// Manages the lifecycle of all connector instances.

type Manager struct {
	mu         sync.RWMutex
	connectors map[string]Connector // keyed by connector ID
	configs    map[string]ConnectorConfig
	results    map[string]*SyncResult // last sync result per connector
}

func NewManager() *Manager {
	return &Manager{
		connectors: make(map[string]Connector),
		configs:    make(map[string]ConnectorConfig),
		results:    make(map[string]*SyncResult),
	}
}

// Register creates and registers a new connector from configuration.
func (m *Manager) Register(ctx context.Context, config ConnectorConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if config.ID == "" {
		config.ID = fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}
	if config.Status == "" {
		config.Status = ConnectorStatusDisconnected
	}
	if config.SyncIntervalMinutes <= 0 {
		config.SyncIntervalMinutes = 60
	}

	conn, err := m.createConnector(config)
	if err != nil {
		return "", fmt.Errorf("manager: create connector: %w", err)
	}

	if err := conn.Configure(config); err != nil {
		return "", fmt.Errorf("manager: configure connector: %w", err)
	}

	m.connectors[config.ID] = conn
	m.configs[config.ID] = config
	m.results[config.ID] = nil

	log.Printf("[CONNECTOR] Registered: %s (%s) as %s", config.Name, config.Type, config.ID)
	return config.ID, nil
}

// Unregister removes a connector from the manager.
func (m *Manager) Unregister(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connectors[id]; ok {
		conn.Disconnect(ctx)
	}

	delete(m.connectors, id)
	delete(m.configs, id)
	delete(m.results, id)
	return nil
}

// Connect establishes a connection to a registered connector.
func (m *Manager) Connect(ctx context.Context, id string) error {
	m.mu.RLock()
	conn, ok := m.connectors[id]
	config := m.configs[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("manager: connector not found: %s", id)
	}

	if err := conn.Connect(ctx); err != nil {
		config.Status = ConnectorStatusError
		m.updateConfig(config)
		return err
	}

	config.Status = ConnectorStatusConnected
	m.updateConfig(config)
	return nil
}

// Disconnect disconnects a connector.
func (m *Manager) Disconnect(ctx context.Context, id string) error {
	m.mu.RLock()
	conn, ok := m.connectors[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("manager: connector not found: %s", id)
	}

	conn.Disconnect(ctx)
	config := m.configs[id]
	config.Status = ConnectorStatusDisconnected
	m.updateConfig(config)
	return nil
}

// GetConnector returns a connector by ID (for direct use).
func (m *Manager) GetConnector(id string) (Connector, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connectors[id]
	if !ok {
		return nil, fmt.Errorf("manager: connector not found: %s", id)
	}
	return conn, nil
}

// GetConfig returns the config for a connector.
func (m *Manager) GetConfig(id string) (ConnectorConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	config, ok := m.configs[id]
	if !ok {
		return ConnectorConfig{}, fmt.Errorf("manager: connector not found: %s", id)
	}
	return config, nil
}

// List returns all registered connectors with their configs and statuses.
func (m *Manager) List() []ConnectorConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configs := make([]ConnectorConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		configs = append(configs, cfg)
	}
	return configs
}

// SyncUsers performs a full sync of users from the connector.
func (m *Manager) SyncUsers(ctx context.Context, id string) (*SyncResult, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	config := m.configs[id]
	config.Status = ConnectorStatusSyncing
	m.updateConfig(config)

	result := &SyncResult{
		ConnectorID:   id,
		ConnectorName: config.Name,
		ConnectorType: string(config.Type),
		StartedAt:     time.Now(),
	}

	// Ensure connected
	if config.Status != ConnectorStatusConnected {
		if err := conn.Connect(ctx); err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Success = false
			result.CompletedAt = time.Now()
			config.Status = ConnectorStatusError
			m.updateConfig(config)
			m.results[id] = result
			return result, err
		}
	}

	// List users from target system
	remoteUsers, err := conn.ListUsers(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("list users: %v", err))
		result.Success = false
		result.CompletedAt = time.Now()
		config.Status = ConnectorStatusError
		config.LastError = err.Error()
		m.updateConfig(config)
		m.results[id] = result
		return result, err
	}

	now := time.Now()
	result.UsersTotal = len(remoteUsers)
	config.LastSyncAt = &now
	config.Status = ConnectorStatusConnected
	config.LastError = ""
	result.Success = true
	result.CompletedAt = now

	m.updateConfig(config)
	m.results[id] = result

	log.Printf("[CONNECTOR] Sync complete for %s: %d users", config.Name, len(remoteUsers))
	return result, nil
}

// SyncGroups performs a full sync of groups from the connector.
func (m *Manager) SyncGroups(ctx context.Context, id string) (*SyncResult, error) {
	conn, err := m.GetConnector(id)
	if err != nil {
		return nil, err
	}

	remoteGroups, err := conn.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("manager: sync groups: %w", err)
	}

	config := m.configs[id]
	result := &SyncResult{
		ConnectorID:   id,
		ConnectorName: config.Name,
		ConnectorType: string(config.Type),
		StartedAt:     time.Now(),
		CompletedAt:   time.Now(),
		GroupsTotal:   len(remoteGroups),
		Success:       true,
	}

	m.results[id] = result
	log.Printf("[CONNECTOR] Group sync complete for %s: %d groups", config.Name, len(remoteGroups))
	return result, nil
}

// GetLastSyncResult returns the last sync result for a connector.
func (m *Manager) GetLastSyncResult(id string) *SyncResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.results[id]
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

// createConnector instantiates the appropriate connector for the given config.
func (m *Manager) createConnector(config ConnectorConfig) (Connector, error) {
	return NewConnector(config.Type)
}

func (m *Manager) updateConfig(config ConnectorConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[config.ID] = config
}

// NewConnector creates a connector of the given type.
func NewConnector(connType ConnectorType) (Connector, error) {
	switch connType {
	case ConnectorTypeEntraID:
		return NewEntraConnector(), nil
	case ConnectorTypeLDAP, ConnectorTypeAD:
		return NewLDAPConnector(), nil
	case ConnectorTypeSCIM:
		return NewSCIMConnector(), nil
	case ConnectorTypeOkta:
		// Okta uses the SCIM connector with Okta-specific SCIM endpoint
		return NewSCIMConnector(), nil
	case ConnectorTypeGeneric:
		return NewSCIMConnector(), nil
	default:
		return nil, fmt.Errorf("unknown connector type: %s", connType)
	}
}
