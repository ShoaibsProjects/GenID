package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event represents an outbox event ready to be processed.
type Event struct {
	ID            string          `json:"id"`
	EventType     string          `json:"event_type"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	Payload       json.RawMessage `json:"payload"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	RetryCount    int             `json:"retry_count"`
	ErrorMessage  string          `json:"error_message,omitempty"`
}

// Outbox provides transactional outbox functionality.
// It ensures that events are inserted atomically with the main database operation.
type Outbox struct {
	pgPool *pgxpool.Pool
}

// NewOutbox creates a new Outbox instance.
func NewOutbox(pgPool *pgxpool.Pool) *Outbox {
	return &Outbox{pgPool: pgPool}
}

// Publish adds an event to the outbox within the provided transaction.
// This ensures atomicity: the main operation and outbox insert succeed or fail together.
//
// Usage:
//
//	tx, _ := s.pgPool.Begin(ctx)
//	defer tx.Rollback(ctx)
//	// ... main operation ...
//	outbox.Publish(ctx, tx, "identity.created", "identity", id, payload, metadata)
//	tx.Commit(ctx)
func (o *Outbox) Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string, payload, metadata any) error {
	if tx == nil {
		return fmt.Errorf("outbox: transaction is nil")
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: marshal payload: %w", err)
	}

	var metadataJSON []byte
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("outbox: marshal metadata: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_type, aggregate_type, aggregate_id, payload, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, eventType, aggregateType, aggregateID, payloadJSON, metadataJSON)

	if err != nil {
		return fmt.Errorf("outbox: insert event: %w", err)
	}

	return nil
}

// GetUnprocessed fetches events that haven't been processed yet, ordered by creation time.
// It respects retry backoff: events with retry_count > 0 are only fetched after
// an exponential delay (1h * 2^retry_count).
func (o *Outbox) GetUnprocessed(ctx context.Context, limit int) ([]Event, error) {
	rows, err := o.pgPool.Query(ctx, `
		SELECT id, event_type, aggregate_type, aggregate_id, payload, metadata, created_at, retry_count, COALESCE(error_message, '')
		FROM outbox_events
		WHERE processed = FALSE
		  AND (retry_count = 0 OR created_at > NOW() - INTERVAL '1 hour' * POWER(2, LEAST(retry_count, 5)))
		  AND expires_at > NOW()
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: query unprocessed: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		err := rows.Scan(&e.ID, &e.EventType, &e.AggregateType, &e.AggregateID,
			&e.Payload, &e.Metadata, &e.CreatedAt, &e.RetryCount, &e.ErrorMessage)
		if err != nil {
			return nil, fmt.Errorf("outbox: scan event: %w", err)
		}
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: rows error: %w", err)
	}

	return events, nil
}

// MarkProcessed marks an event as successfully processed.
func (o *Outbox) MarkProcessed(ctx context.Context, id string) error {
	_, err := o.pgPool.Exec(ctx, `
		UPDATE outbox_events
		SET processed = TRUE, processed_at = NOW(), error_message = NULL
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("outbox: mark processed: %w", err)
	}
	return nil
}

// MarkFailed marks an event as failed and increments the retry count.
func (o *Outbox) MarkFailed(ctx context.Context, id string, errMsg string) error {
	_, err := o.pgPool.Exec(ctx, `
		UPDATE outbox_events
		SET retry_count = retry_count + 1,
		    error_message = $2,
		    created_at = CASE WHEN retry_count = 0 THEN created_at ELSE NOW() END
		WHERE id = $1 AND processed = FALSE
	`, id, errMsg)
	if err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	return nil
}

// Stats returns outbox queue statistics.
func (o *Outbox) Stats(ctx context.Context) (map[string]int64, error) {
	var pending, failed, total int64

	err := o.pgPool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE processed = FALSE) as pending,
			COUNT(*) FILTER (WHERE processed = FALSE AND retry_count > 0) as failed,
			COUNT(*) FILTER (WHERE expires_at > NOW()) as total
		FROM outbox_events
	`).Scan(&pending, &failed, &total)

	if err != nil {
		return nil, fmt.Errorf("outbox: stats query: %w", err)
	}

	return map[string]int64{
		"pending": pending,
		"failed":  failed,
		"total":   total,
	}, nil
}
