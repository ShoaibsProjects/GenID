package connector

import "time"

// ─── Connector Types ─────────────────────────────────────────

// ConnectorType represents the type of directory/identity connector.
type ConnectorType string

const (
	ConnectorTypeEntraID  ConnectorType = "entra_id"
	ConnectorTypeLDAP     ConnectorType = "ldap"
	ConnectorTypeAD       ConnectorType = "active_directory"
	ConnectorTypeSCIM     ConnectorType = "scim"
	ConnectorTypeOkta     ConnectorType = "okta"
	ConnectorTypeAWS      ConnectorType = "aws_iam"
	ConnectorTypeGCP      ConnectorType = "gcp_iam"
	ConnectorTypeGeneric  ConnectorType = "generic"
)

// ConnectorStatus represents the current state of a connector.
type ConnectorStatus string

const (
	ConnectorStatusDisconnected    ConnectorStatus = "disconnected"
	ConnectorStatusConnected       ConnectorStatus = "connected"
	ConnectorStatusError           ConnectorStatus = "error"
	ConnectorStatusSyncing         ConnectorStatus = "syncing"
	ConnectorStatusDegraded        ConnectorStatus = "degraded"
)

// ConnectorConfig holds the configuration for a connector instance.
type ConnectorConfig struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	Name       string            `json:"name"`
	Type       ConnectorType     `json:"type"`
	Status     ConnectorStatus   `json:"status"`
	Endpoint   string            `json:"endpoint,omitempty"`    // URL or host:port
	AuthType   string            `json:"auth_type,omitempty"`   // oauth2, basic, api_key, certificate, kerberos
	Username   string            `json:"username,omitempty"`
	Password   string            `json:"password,omitempty"`    // encrypted in storage
	ClientID   string            `json:"client_id,omitempty"`
	ClientSecret string          `json:"client_secret,omitempty"`
	TokenURL   string            `json:"token_url,omitempty"`
	TenantName string            `json:"tenant_name,omitempty"` // Entra ID tenant name
	BaseDN     string            `json:"base_dn,omitempty"`     // LDAP base DN
	Domain     string            `json:"domain,omitempty"`      // AD domain
	Cert       string            `json:"cert,omitempty"`        // PEM cert for mutual TLS
	Properties map[string]string `json:"properties,omitempty"`  // extensible properties
	SyncIntervalMinutes int     `json:"sync_interval_minutes"`  // default 60
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	LastSyncAt *time.Time       `json:"last_sync_at,omitempty"`
	LastError  string           `json:"last_error,omitempty"`
}

// ─── Connector Objects ───────────────────────────────────────

// ConnectorUser represents a user/identity pulled from a target system.
type ConnectorUser struct {
	ExternalID    string            `json:"external_id"`     // ID in the target system
	Username      string            `json:"username"`
	Email         string            `json:"email"`
	DisplayName   string            `json:"display_name"`
	FirstName     string            `json:"first_name,omitempty"`
	LastName      string            `json:"last_name,omitempty"`
	Department    string            `json:"department,omitempty"`
	Manager       string            `json:"manager,omitempty"`     // manager's ExternalID
	Title         string            `json:"title,omitempty"`
	Phone         string            `json:"phone,omitempty"`
	Mobile        string            `json:"mobile,omitempty"`
	StreetAddress string            `json:"street_address,omitempty"`
	City          string            `json:"city,omitempty"`
	State         string            `json:"state,omitempty"`
	ZipCode       string            `json:"zip_code,omitempty"`
	Country       string            `json:"country,omitempty"`
	EmployeeID    string            `json:"employee_id,omitempty"`
	CostCenter    string            `json:"cost_center,omitempty"`
	Division      string            `json:"division,omitempty"`
	Company       string            `json:"company,omitempty"`
	Enabled       bool              `json:"enabled"`
	Locked        bool              `json:"locked,omitempty"`
	Groups        []string          `json:"groups,omitempty"`      // group ExternalIDs
	Roles         []string          `json:"roles,omitempty"`       // role names
	Attributes    map[string]string `json:"attributes,omitempty"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

// ConnectorGroup represents a group/role pulled from a target system.
type ConnectorGroup struct {
	ExternalID    string            `json:"external_id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Type          string            `json:"type,omitempty"`        // security, distribution, mail-enabled
	Scope         string            `json:"scope,omitempty"`       // global, domain_local, universal
	Members       []string          `json:"members,omitempty"`     // member ExternalIDs
	Attributes    map[string]string `json:"attributes,omitempty"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

// ─── Sync Results ────────────────────────────────────────────

// SyncResult holds the result of a connector synchronization.
type SyncResult struct {
	ConnectorID    string          `json:"connector_id"`
	ConnectorName  string          `json:"connector_name"`
	ConnectorType  string          `json:"connector_type"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    time.Time       `json:"completed_at"`
	UsersCreated   int             `json:"users_created"`
	UsersUpdated   int             `json:"users_updated"`
	UsersDeleted   int             `json:"users_deleted"`
	UsersTotal     int             `json:"users_total"`
	Users          []ConnectorUser `json:"users,omitempty"`
	GroupsCreated  int             `json:"groups_created"`
	GroupsUpdated  int             `json:"groups_updated"`
	GroupsDeleted  int             `json:"groups_deleted"`
	GroupsTotal    int             `json:"groups_total"`
	Errors         []string        `json:"errors,omitempty"`
	Success        bool            `json:"success"`
}

// ProvisioningRequest represents a request to provision an identity to a target system.
type ProvisioningRequest struct {
	ConnectorID    string `json:"connector_id"`
	IdentityID     string `json:"identity_id"`
	Action         string `json:"action"`          // create, update, delete, enable, disable
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Department     string `json:"department,omitempty"`
	PendingGroups  []string `json:"pending_groups,omitempty"`
	PendingRoles   []string `json:"pending_roles,omitempty"`
}
