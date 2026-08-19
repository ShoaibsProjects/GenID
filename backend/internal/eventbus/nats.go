package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// Event represents a domain event published to the event bus.
type Event struct {
	ID          string          `json:"id"`
	EventType   string          `json:"event_type"`
	AggregateID string          `json:"aggregate_id"`
	TenantID    string          `json:"tenant_id"`
	Payload     json.RawMessage `json:"payload"`
	Timestamp   time.Time       `json:"timestamp"`
}

// NatsBus is a NATS JetStream event bus implementation.
type NatsBus struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	stream string
}

// NewNatsBus creates a new NATS event bus connection.
func NewNatsBus(natsURL string) (*NatsBus, error) {
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
			log.Printf("[NATS] disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			log.Printf("[NATS] reconnected to %s", conn.ConnectedUrl())
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}

	bus := &NatsBus{
		nc:     nc,
		js:     js,
		stream: "genid-events",
	}

	// Ensure the stream exists
	if err := bus.ensureStream(); err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats ensure stream: %w", err)
	}

	log.Printf("[NATS] connected to %s, stream: %s", natsURL, bus.stream)
	return bus, nil
}

// ensureStream creates the JetStream stream if it doesn't exist.
func (b *NatsBus) ensureStream() error {
	_, err := b.js.AddStream(&nats.StreamConfig{
		Name:      b.stream,
		Subjects:  []string{b.stream + ".>"},
		Storage:   nats.FileStorage,
		MaxMsgs:   100000,
		MaxAge:    24 * time.Hour,
		Retention: nats.LimitsPolicy,
	})
	return err
}

// Publish publishes an event to the NATS JetStream.
// Non-blocking: logs errors but does not fail the caller.
func (b *NatsBus) Publish(ctx context.Context, event Event) error {
	if b == nil || b.js == nil {
		return nil // NATS not configured
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[NATS] marshal error: %v", err)
		return nil // non-blocking
	}

	subject := fmt.Sprintf("%s.%s", b.stream, event.EventType)

	_, err = b.js.Publish(subject, data)
	if err != nil {
		log.Printf("[NATS] publish error (subject=%s): %v", subject, err)
		return nil // non-blocking — PG/Neo4j is source of truth
	}

	log.Printf("[NATS] published event %s (id=%s)", event.EventType, event.ID)
	return nil
}

// JetStream exposes the JetStream context for consumers that need to subscribe.
func (b *NatsBus) JetStream() nats.JetStreamContext {
	return b.js
}

// Conn exposes the underlying NATS connection.
func (b *NatsBus) Conn() *nats.Conn {
	return b.nc
}

// StreamName returns the stream name used by this bus.
func (b *NatsBus) StreamName() string {
	return b.stream
}

// Close gracefully closes the NATS connection.
func (b *NatsBus) Close() {
	if b != nil && b.nc != nil {
		b.nc.Close()
		log.Printf("[NATS] connection closed")
	}
}
