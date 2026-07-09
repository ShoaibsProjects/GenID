package domain

import (
	"time"
)

// ─── Core Identity Types ───────────────────────────────────

type IdentityType string

const (
	IdentityTypeHuman          IdentityType = "human"
	IdentityTypeServiceAccount IdentityType = "service_account"
	IdentityTypeAIAgent        IdentityType = "ai_agent"
	IdentityTypeRobot          IdentityType = "robot"
	IdentityTypeIoTDevice      IdentityType = "iot_device"
	IdentityTypeRPABot         IdentityType = "rpa_bot"
	IdentityTypeAPIKey         IdentityType = "api_key"
)

type IdentityStatus string

const (
	StatusActive        IdentityStatus = "active"
	StatusInactive      IdentityStatus = "inactive"
	StatusSuspended     IdentityStatus = "suspended"
	StatusTerminated    IdentityStatus = "terminated"
	StatusRevoked       IdentityStatus = "revoked"
	StatusPendingReview IdentityStatus = "pending_review"
)

type EntityType string

const (
	EntityHuman          EntityType = "Identity"
	EntityNonHuman       EntityType = "NonHumanIdentity"
	EntityEntitlement    EntityType = "Entitlement"
	EntityRole           EntityType = "Role"
	EntityResource       EntityType = "Resource"
	EntitySession        EntityType = "Session"
	EntityPolicy         EntityType = "Policy"
	EntityCAEPEvent      EntityType = "CAEPEvent"
)

// ─── Identity ─────────────────────────────────────────────

type Identity struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	Type           IdentityType      `json:"type"`
	Status         IdentityStatus    `json:"status"`
	Email          string            `json:"email"`
	DisplayName    string            `json:"display_name"`
	Department     string            `json:"department"`
	EmployeeID     string            `json:"employee_id"`
	ManagerID      string            `json:"manager_id"`
	RiskScore      float64           `json:"risk_score"`
	RiskFactors    []string          `json:"risk_factors"`
	AssuranceLevel string            `json:"assurance_level"`
	Attributes     map[string]string `json:"attributes"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	LastAccessedAt *time.Time        `json:"last_accessed_at"`
	LastReviewedAt *time.Time        `json:"last_reviewed_at"`
}

// ─── Non-Human Identity (NHI / AI Agent) ─────────────────

type NonHumanIdentity struct {
	ID                   string            `json:"id"`
	TenantID             string            `json:"tenant_id"`
	Name                 string            `json:"name"`
	Type                 IdentityType      `json:"type"`
	Status               IdentityStatus    `json:"status"`
	AgentCardID          string            `json:"agent_card_id"`
	Protocols            []string          `json:"protocols"`
	OwnerID              string            `json:"owner_id"`
	TeamID               string            `json:"team_id"`
	CreatedBy            string            `json:"created_by"`
	DeploymentEnv        string            `json:"deployment_environment"`
	LastRotatedAt        *time.Time        `json:"last_rotated_at"`
	ExpiresAt            *time.Time        `json:"expires_at"`
	RiskScore            float64           `json:"risk_score"`
	IsGoverned           bool              `json:"is_governed"`
	Framework            string            `json:"framework"`
	Capabilities         []string          `json:"capabilities"`
	Attributes           map[string]string `json:"attributes"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// ─── Entitlement (Permission) ─────────────────────────────

type Entitlement struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenant_id"`
	AppName            string            `json:"app_name"`
	PermissionLevel    string            `json:"permission_level"`
	EntitlementType    string            `json:"entitlement_type"`
	IsToxic            bool              `json:"is_toxic"`
	RiskClassification string            `json:"risk_classification"`
	LastUsedAt         *time.Time        `json:"last_used_at"`
	UsageCount90d      int               `json:"usage_count_90d"`
	IsRubberband       bool              `json:"is_rubberband"`
	Attributes         map[string]string `json:"attributes"`
}

// ─── Resource (Target System) ─────────────────────────────

type Resource struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenant_id"`
	Name               string            `json:"name"`
	Type               string            `json:"type"`
	Criticality        string            `json:"criticality"`
	DataClassification string            `json:"data_classification"`
	OwnerTeam          string            `json:"owner_team"`
	ConnectionType     string            `json:"connection_type"`
	HealthStatus       string            `json:"health_status"`
	Attributes         map[string]string `json:"attributes"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// ─── Role ─────────────────────────────────────────────────

type Role struct {
	ID                string            `json:"id"`
	TenantID          string            `json:"tenant_id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	RoleType          string            `json:"role_type"`
	IsAutoAssigned    bool              `json:"is_auto_assigned"`
	ApprovalRequired  bool              `json:"approval_required"`
	MaxDurationHours  int               `json:"max_duration_hours"`
	CreatedByRolmining bool             `json:"created_by_rolmining"`
	ConfidenceScore   float64           `json:"confidence_score"`
	IsActive          bool              `json:"is_active"`
	Attributes        map[string]string `json:"attributes"`
	CreatedAt         time.Time         `json:"created_at"`
}

// ─── Session ──────────────────────────────────────────────

type Session struct {
	ID             string    `json:"id"`
	IdentityID     string    `json:"identity_id"`
	TenantID       string    `json:"tenant_id"`
	AuthMethod     string    `json:"auth_method"`
	AssuranceLevel string    `json:"assurance_level"`
	DeviceID       string    `json:"device_id"`
	IPAddress      string    `json:"ip_address"`
	UserAgent      string    `json:"user_agent"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// ─── Agent Card (A2A/MCP Identity Document) ───────────────

type AgentCard struct {
	AgentID         string   `json:"agent_id"`
	AgentType       string   `json:"agent_type"`
	OwnerID         string   `json:"owner_id"`
	TeamID          string   `json:"team_id"`
	Capabilities    []string `json:"capabilities"`
	DeploymentEnv   string   `json:"deployment_env"`
	Protocols       []string `json:"protocols"`
	PublicKey       string   `json:"public_key"`
	IssuedAt        int64    `json:"issued_at"`
	ExpiresAt       int64    `json:"expires_at"`
	SignatureScheme string   `json:"signature_scheme"`
	Signature       []byte   `json:"signature"`
}

// ─── Delegation Chain ─────────────────────────────────────

type DelegationChain struct {
	ID               string    `json:"id"`
	ParentIdentityID string    `json:"parent_identity_id"`
	ChildIdentityID  string    `json:"child_identity_id"`
	ScopeNarrowing   []string  `json:"scope_narrowing"`
	MaxDepthRemaining int      `json:"max_depth_remaining"`
	DelegatedAt      time.Time `json:"delegated_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	IsActive         bool      `json:"is_active"`
}

// ─── CAEP Event ───────────────────────────────────────────

type CAEPEvent struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	EventType        string    `json:"event_type"`
	EventJTI         string    `json:"event_jti"`
	IdentityID       string    `json:"identity_id"`
	SessionID        string    `json:"session_id"`
	InitiatingEntity string    `json:"initiating_entity"`
	ReasonAdmin      string    `json:"reason_admin"`
	ReasonUser       string    `json:"reason_user"`
	Payload          []byte    `json:"payload"`
	DeliveryStatus   string    `json:"delivery_status"`
	DeliveredTo      []string  `json:"delivered_to"`
	CreatedAt        time.Time `json:"created_at"`
	DeliveredAt      *time.Time `json:"delivered_at"`
}

// ─── Cedar Policy ─────────────────────────────────────────

type CedarPolicy struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	PolicyID     string    `json:"policy_id"`
	Effect       string    `json:"effect"`  // "permit" or "forbid"
	PolicySource string    `json:"policy_source"`
	IsActive     bool      `json:"is_active"`
	Version      int       `json:"version"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// ─── Audit Record ─────────────────────────────────────────

type AuditRecord struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	EventType     string            `json:"event_type"`
	ActorID       string            `json:"actor_id"`
	ActorType     string            `json:"actor_type"`
	TargetID      string            `json:"target_id"`
	TargetType    string            `json:"target_type"`
	Action        string            `json:"action"`
	Resource      string            `json:"resource"`
	Details       map[string]any    `json:"details"`
	IPAddress     string            `json:"ip_address"`
	CorrelationID string            `json:"correlation_id"`
	TraceID       string            `json:"trace_id"`
	CreatedAt     time.Time         `json:"created_at"`
}
