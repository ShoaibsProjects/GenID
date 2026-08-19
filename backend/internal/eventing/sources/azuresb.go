package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/google/uuid"
	"github.com/observeid/genid/internal/eventbus"
)

// AzureSBConfig holds connection details for an Azure Service Bus edge
// adapter. Enabled only when both fields are set (via environment); the
// platform runs without Azure entirely when they are absent.
type AzureSBConfig struct {
	ConnectionString string // AZURE_SERVICEBUS_CONNECTION_STRING
	Queue            string // AZURE_SERVICEBUS_QUEUE (queue or topic name)
	Subscription     string // AZURE_SERVICEBUS_SUBSCRIPTION (optional, for topics)
	// SourceName selects the normalization config from the registry
	// (default "entra" — the common case is Entra ID Protection events
	// landing on a Service Bus queue via Event Grid).
	SourceName string // AZURE_SERVICEBUS_SOURCE
}

// AzureSBAdapter subscribes to an Azure Service Bus queue/subscription and
// republishes normalized events onto the internal NATS fabric. It is the
// reference implementation of ADR-001's "brokers at the edge" rule: ASB is
// the customer's world, NATS is ours, and this adapter is the only thing
// that knows about both.
type AzureSBAdapter struct {
	cfg    AzureSBConfig
	source *SourceConfig
	bus    *eventbus.NatsBus
	client *azservicebus.Client
}

// NewAzureSBAdapter builds the adapter. Returns an error when the client
// cannot be constructed; connection problems surface in Run.
func NewAzureSBAdapter(cfg AzureSBConfig, registry *Registry, bus *eventbus.NatsBus) (*AzureSBAdapter, error) {
	if cfg.ConnectionString == "" || cfg.Queue == "" {
		return nil, fmt.Errorf("azuresb: connection string and queue are required")
	}
	sourceName := cfg.SourceName
	if sourceName == "" {
		sourceName = "entra"
	}
	src := registry.Get(sourceName)
	if src == nil {
		return nil, fmt.Errorf("azuresb: source %q not found in registry", sourceName)
	}
	client, err := azservicebus.NewClientFromConnectionString(cfg.ConnectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("azuresb client: %w", err)
	}
	return &AzureSBAdapter{cfg: cfg, source: src, bus: bus, client: client}, nil
}

// Run receives messages until ctx is cancelled. Receiver options keep the
// adapter simple: one message at a time, default settlement (Complete on
// success, Abandon on failure → Service Bus redelivers).
func (a *AzureSBAdapter) Run(ctx context.Context) error {
	var receiver *azservicebus.Receiver
	var err error
	if a.cfg.Subscription != "" {
		receiver, err = a.client.NewReceiverForSubscription(a.cfg.Queue, a.cfg.Subscription, nil)
	} else {
		receiver, err = a.client.NewReceiverForQueue(a.cfg.Queue, nil)
	}
	if err != nil {
		return fmt.Errorf("azuresb receiver: %w", err)
	}
	defer receiver.Close(ctx)

	log.Printf("[AZURESB] listening queue=%s subscription=%s source=%s",
		a.cfg.Queue, a.cfg.Subscription, a.source.Name)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		msgs, err := receiver.ReceiveMessages(ctx, 10, nil)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("[AZURESB] receive error: %v", err)
			time.Sleep(5 * time.Second) // backoff before reconnect attempt
			continue
		}

		for _, msg := range msgs {
			if err := a.handleMessage(ctx, msg); err != nil {
				log.Printf("[AZURESB] message %s failed: %v — abandoning", msg.MessageID, err)
				_ = receiver.AbandonMessage(ctx, msg, nil)
				continue
			}
			_ = receiver.CompleteMessage(ctx, msg, nil)
		}
	}
}

func (a *AzureSBAdapter) handleMessage(ctx context.Context, msg *azservicebus.ReceivedMessage) error {
	var payload map[string]any
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		// Poison payload — complete it so it doesn't redeliver forever.
		log.Printf("[AZURESB] dropping non-JSON message %s", msg.MessageID)
		return nil
	}

	norm, err := a.source.Normalize(payload)
	if err != nil {
		log.Printf("[AZURESB] dropping unmappable message %s: %v", msg.MessageID, err)
		return nil
	}

	raw, _ := json.Marshal(payload)
	enriched, _ := json.Marshal(map[string]any{
		"identity_id": norm.IdentityID,
		"source":      norm.Source,
		"severity":    norm.Severity,
		"raw":         json.RawMessage(raw),
	})

	return a.bus.Publish(ctx, eventbus.Event{
		ID:          uuid.New().String(),
		EventType:   norm.EventType,
		AggregateID: norm.IdentityID,
		TenantID:    "00000000-0000-0000-0000-000000000001",
		Payload:     enriched,
		Timestamp:   time.Now().UTC(),
	})
}

// Close releases the underlying Service Bus client.
func (a *AzureSBAdapter) Close(ctx context.Context) error {
	if a.client != nil {
		return a.client.Close(ctx)
	}
	return nil
}
