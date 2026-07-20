package connector

import (
	"context"
	"time"
)

// ─── EntityEvent ──────────────────────────────────────────────
// An EntityEvent represents a single change to an entity in an
// external identity source. Every connector produces these events.
// The platform consumes them to sync state.
//
// This replaces the fixed-return-style methods (ListUsers returning
// []ConnectorUser) with a streaming event model. A connector that
// implements ConnectorV2 produces EntityEvent streams instead of
// returning batch struct slices.

type EventOperation string

const (
	EventCreated EventOperation = "created"
	EventUpdated EventOperation = "updated"
	EventDeleted EventOperation = "deleted"
)

type EntityEvent struct {
	// EntityType identifies what kind of entity changed.
	// "User", "Group", "Entitlement", "Resource", or any custom type.
	EntityType string `json:"entity_type"`

	// Operation tells the platform what happened.
	Operation EventOperation `json:"operation"`

	// Key is the external ID of the entity in the source system.
	Key string `json:"key"`

	// Data contains the current state of the entity.
	// For "created" and "updated" operations, this is the full record.
	// For "deleted" operations, this may be empty or contain just the key.
	Data map[string]any `json:"data,omitempty"`

	// Timestamp is when the change occurred in the source system.
	Timestamp time.Time `json:"timestamp"`

	// Metadata carries connector-specific context:
	//   "connector_id" — which connector produced this
	//   "sync_batch"   — unique ID for this sync run
	//   "delta_token"  — cursor for incremental sync (if supported)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SyncOptions controls how a connector produces its event stream.
type SyncOptions struct {
	// SyncMode: "full" = re-read everything, "delta" = only changes since last token
	SyncMode string `json:"sync_mode,omitempty"`

	// DeltaToken is the cursor from the previous delta sync.
	// Connectors that support delta sync should resume from this point.
	DeltaToken string `json:"delta_token,omitempty"`

	// Fields limits which fields are requested from the source.
	// Empty means request all fields the connector supports.
	Fields []string `json:"fields,omitempty"`

	// BatchSize hints at how many events to include per batch/channel write.
	BatchSize int `json:"batch_size,omitempty"`
}

// ─── ConnectorV2 ───────────────────────────────────────────────
// ConnectorV2 is an OPTIONAL extension of the Connector interface.
// Connectors that implement it replace the 24 fixed methods with
// 2 event-oriented methods: Events and Apply.
//
// The Manager checks whether a connector implements ConnectorV2.
// If yes, it uses the event path (streaming, dynamic entity types).
// If no, it falls back to the V1 fixed-interface path.
//
// This is the SAME PATTERN Go uses for io.WriterTo and io.ReaderFrom:
// optional interfaces that provide a more efficient path when available.

type ConnectorV2 interface {
	Connector

	// Events produces a stream of entity events from the external source.
	// The connector reads from its configured source and pushes events
	// into the returned channel. The platform reads from this channel
	// and processes each event.
	//
	// The channel is closed by the connector when the sync is complete.
	// The platform should consume events until the channel is closed
	// or the context is cancelled.
	//
	// For full syncs: the connector reads every entity and sends one
	// "created" event per entity (the platform reconciles deletions).
	// For delta syncs: the connector sends only what changed, using
	// "created", "updated", and "deleted" operations as appropriate.
	// The final event should include the delta token in its metadata.
	Events(ctx context.Context, opts SyncOptions) (<-chan EntityEvent, error)

	// Apply sends entity changes TO the external source.
	// The events are mutations (create, update, delete) that should be
	// applied to the source system in order.
	//
	// Returns the list of applied events with their results.
	// Error indicates a connection-level failure (not a per-event failure).
	Apply(ctx context.Context, events []EntityEvent) ([]ApplyResult, error)
}

// ApplyResult reports the outcome of a single Apply operation.
type ApplyResult struct {
	Key     string `json:"key"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
