package connector

import "time"

// ─── Connector Types ─────────────────────────────────────────

type ConnectorType string

const (
	ConnectorTypeEntraID ConnectorType = "entra_id"
	ConnectorTypeLDAP    ConnectorType = "ldap"
	ConnectorTypeAD      ConnectorType = "active_directory"
	ConnectorTypeSCIM    ConnectorType = "scim"
	ConnectorTypeOkta    ConnectorType = "okta"
	ConnectorTypeAWS     ConnectorType = "aws_iam"
	ConnectorTypeGCP     ConnectorType = "gcp_iam"
	ConnectorTypeGeneric ConnectorType = "generic"
)

type ConnectorStatus string

const (
	ConnectorStatusDisconnected ConnectorStatus = "disconnected"
	ConnectorStatusConnected    ConnectorStatus = "connected"
	ConnectorStatusError        ConnectorStatus = "error"
	ConnectorStatusSyncing      ConnectorStatus = "syncing"
	ConnectorStatusDegraded     ConnectorStatus = "degraded"
)

// ─── Connector Configuration ─────────────────────────────────

type ConnectorConfig struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenant_id"`
	Name               string            `json:"name"`
	Type               ConnectorType     `json:"type"`
	Status             ConnectorStatus   `json:"status"`
	Endpoint           string            `json:"endpoint,omitempty"`
	AuthType           string            `json:"auth_type,omitempty"`
	Username           string            `json:"username,omitempty"`
	Password           string            `json:"password,omitempty"`
	ClientID           string            `json:"client_id,omitempty"`
	ClientSecret       string            `json:"client_secret,omitempty"`
	TokenURL           string            `json:"token_url,omitempty"`
	TenantName         string            `json:"tenant_name,omitempty"`
	BaseDN             string            `json:"base_dn,omitempty"`
	Domain             string            `json:"domain,omitempty"`
	Cert               string            `json:"cert,omitempty"`
	Properties         map[string]string `json:"properties,omitempty"`
	SyncIntervalMinutes int              `json:"sync_interval_minutes"`
	DeltaToken         string            `json:"delta_token,omitempty"`
	Watermark          string            `json:"watermark,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	LastSyncAt         *time.Time        `json:"last_sync_at,omitempty"`
	LastError          string            `json:"last_error,omitempty"`
}

// ─── Connector Objects ───────────────────────────────────────

type ConnectorUser struct {
	ExternalID    string            `json:"external_id"`
	Username      string            `json:"username"`
	Email         string            `json:"email"`
	DisplayName   string            `json:"display_name"`
	FirstName     string            `json:"first_name,omitempty"`
	LastName      string            `json:"last_name,omitempty"`
	Department    string            `json:"department,omitempty"`
	Manager       string            `json:"manager,omitempty"`
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
	Groups        []string          `json:"groups,omitempty"`
	Roles         []string          `json:"roles,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

type ConnectorGroup struct {
	ExternalID  string            `json:"external_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	Members     []string          `json:"members,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

// ─── Sync Results ────────────────────────────────────────────

type SyncResult struct {
	ConnectorID   string          `json:"connector_id"`
	ConnectorName string          `json:"connector_name"`
	ConnectorType string          `json:"connector_type"`
	StartedAt     time.Time       `json:"started_at"`
	CompletedAt   time.Time       `json:"completed_at"`
	UsersCreated  int             `json:"users_created"`
	UsersUpdated  int             `json:"users_updated"`
	UsersDeleted  int             `json:"users_deleted"`
	UsersTotal    int             `json:"users_total"`
	Users         []ConnectorUser `json:"users,omitempty"`
	GroupsCreated int             `json:"groups_created"`
	GroupsUpdated int             `json:"groups_updated"`
	GroupsDeleted int             `json:"groups_deleted"`
	GroupsTotal   int             `json:"groups_total"`
	DeltaToken    string          `json:"delta_token,omitempty"`
	Watermark     string          `json:"watermark,omitempty"`
	Errors        []string        `json:"errors,omitempty"`
	Success       bool            `json:"success"`
}

// ─── Schema Discovery ────────────────────────────────────────

type AttributeSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	MultiValued bool   `json:"multi_valued"`
	SampleValue string `json:"sample_value,omitempty"`
}

type SchemaResult struct {
	ObjectType string            `json:"object_type"`
	Attributes []AttributeSchema `json:"attributes"`
	Count      int               `json:"count"`
}

// ─── Health Monitoring ───────────────────────────────────────

type HealthReport struct {
	ConnectorID        string        `json:"connector_id"`
	ConnectorName      string        `json:"connector_name"`
	Status             string        `json:"status"`
	LastSyncAt         *time.Time    `json:"last_sync_at"`
	LastError          string        `json:"last_error"`
	TotalUsersSynced   int           `json:"total_users_synced"`
	TotalGroupsSynced  int           `json:"total_groups_synced"`
	LastSyncDuration   string        `json:"last_sync_duration,omitempty"`
	ConsecutiveSuccess int           `json:"consecutive_success"`
	ConsecutiveErrors  int           `json:"consecutive_errors"`
	DeltaSupported     bool          `json:"delta_supported"`
	SupportsSchema     bool          `json:"supports_schema"`
}

// ─── Provisioning ────────────────────────────────────────────

type ProvisioningRequest struct {
	ConnectorID   string   `json:"connector_id"`
	IdentityID    string   `json:"identity_id"`
	Action        string   `json:"action"`
	Email         string   `json:"email"`
	DisplayName   string   `json:"display_name"`
	Department    string   `json:"department,omitempty"`
	PendingGroups []string `json:"pending_groups,omitempty"`
	PendingRoles  []string `json:"pending_roles,omitempty"`
}
