package connector

import "context"

// ─── Connector Interface ─────────────────────────────────────
// Every directory/identity provider connector must implement this interface.

type Connector interface {
	// Type returns the connector type identifier.
	Type() ConnectorType

	// Name returns the human-readable connector name.
	Name() string

	// Configure applies the connector configuration.
	Configure(config ConnectorConfig) error

	// Connect tests and establishes a connection to the target system.
	Connect(ctx context.Context) error

	// Disconnect closes the connection.
	Disconnect(ctx context.Context) error

	// TestConnection validates connectivity without persisting the connector.
	TestConnection(ctx context.Context) error

	// GetStatus returns the current connector health status.
	GetStatus(ctx context.Context) ConnectorStatus

	// ─── User Operations ─────────────────────────────────────

	// ListUsers fetches all users from the target system.
	ListUsers(ctx context.Context) ([]ConnectorUser, error)

	// GetUser fetches a single user by external ID.
	GetUser(ctx context.Context, externalID string) (*ConnectorUser, error)

	// GetUserByUsername fetches a user by username/email (for idempotent lookups).
	GetUserByUsername(ctx context.Context, username string) (*ConnectorUser, error)

	// CreateUser creates a user in the target system.
	CreateUser(ctx context.Context, user ConnectorUser) (string, error)

	// UpdateUser updates a user in the target system.
	UpdateUser(ctx context.Context, externalID string, user ConnectorUser) error

	// DeleteUser deletes a user from the target system (hard or soft).
	DeleteUser(ctx context.Context, externalID string) error

	// DisableUser disables/suspends a user.
	DisableUser(ctx context.Context, externalID string) error

	// EnableUser enables a previously disabled user.
	EnableUser(ctx context.Context, externalID string) error

	// ─── Group Operations ────────────────────────────────────

	// ListGroups fetches all groups from the target system.
	ListGroups(ctx context.Context) ([]ConnectorGroup, error)

	// GetGroup fetches a single group by external ID.
	GetGroup(ctx context.Context, externalID string) (*ConnectorGroup, error)

	// CreateGroup creates a group in the target system.
	CreateGroup(ctx context.Context, group ConnectorGroup) (string, error)

	// UpdateGroup updates a group in the target system.
	UpdateGroup(ctx context.Context, externalID string, group ConnectorGroup) error

	// DeleteGroup deletes a group from the target system.
	DeleteGroup(ctx context.Context, externalID string) error

	// AddGroupMember adds a user to a group.
	AddGroupMember(ctx context.Context, groupID, userID string) error

	// RemoveGroupMember removes a user from a group.
	RemoveGroupMember(ctx context.Context, groupID, userID string) error
}
