package connector

import "context"

// ─── Connector Interface ─────────────────────────────────────
// Every directory/identity provider connector must implement this interface.
// This covers the full IAM connector lifecycle: connect, sync, lifecycle, schema.

type Connector interface {
	// ─── Lifecycle ─────────────────────────────────────────

	Type() ConnectorType
	Name() string
	Configure(config ConnectorConfig) error
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	TestConnection(ctx context.Context) error
	GetStatus(ctx context.Context) ConnectorStatus

	// ─── User Operations ──────────────────────────────────

	ListUsers(ctx context.Context) ([]ConnectorUser, error)
	GetUser(ctx context.Context, externalID string) (*ConnectorUser, error)
	GetUserByUsername(ctx context.Context, username string) (*ConnectorUser, error)
	CreateUser(ctx context.Context, user ConnectorUser) (string, error)
	UpdateUser(ctx context.Context, externalID string, user ConnectorUser) error
	DeleteUser(ctx context.Context, externalID string) error
	DisableUser(ctx context.Context, externalID string) error
	EnableUser(ctx context.Context, externalID string) error

	// ─── Group Operations ─────────────────────────────────

	ListGroups(ctx context.Context) ([]ConnectorGroup, error)
	GetGroup(ctx context.Context, externalID string) (*ConnectorGroup, error)
	CreateGroup(ctx context.Context, group ConnectorGroup) (string, error)
	UpdateGroup(ctx context.Context, externalID string, group ConnectorGroup) error
	DeleteGroup(ctx context.Context, externalID string) error
	AddGroupMember(ctx context.Context, groupID, userID string) error
	RemoveGroupMember(ctx context.Context, groupID, userID string) error

	// ─── Delta / Incremental Sync ─────────────────────────
	// Returns users changed since the token. Returns new users + updated token.
	// If not supported, returns ErrDeltaNotSupported.
	ListUsersDelta(ctx context.Context, deltaToken string) (users []ConnectorUser, newToken string, err error)

	// ─── Entitlement Operations ────────────────────────────
	// Lists all entitlements (role assignments, app roles) across the directory.
	// Returns ErrNotSupported if the connector doesn't support entitlement enumeration.
	ListEntitlements(ctx context.Context) ([]ConnectorEntitlement, error)

	// ─── Resource Operations ───────────────────────────────
	// Lists all resources (applications, service principals, devices) across the directory.
	// Returns ErrNotSupported if the connector doesn't support resource enumeration.
	ListResources(ctx context.Context) ([]ConnectorResource, error)

	// ─── Schema Discovery ─────────────────────────────────
	// Discovers the schema/attributes available from the source.
	// Returns nil if not supported.
	DiscoverSchema(ctx context.Context) (*SchemaResult, error)
}

// Sentinel errors for optional connector features
var (
	ErrDeltaNotSupported  = &DeltaNotSupportedError{}
	ErrNotSupported       = &NotSupportedError{}
)

type DeltaNotSupportedError struct{}

func (e *DeltaNotSupportedError) Error() string { return "delta sync not supported by this connector" }

type NotSupportedError struct{}

func (e *NotSupportedError) Error() string { return "feature not supported by this connector" }
