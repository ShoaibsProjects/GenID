# ADR-001: Event Backbone — NATS JetStream Inside, Brokers at the Edge

- **Status:** Accepted
- **Date:** 2026-08-15
- **Deciders:** GenID architecture
- **Context:** Team confusion ("Which hub are we using? Is Neo4j the messaging hub? Why not Kafka?")

## Decision

1. **Internal event fabric = NATS JetStream.** All GenID-internal domain events (`identity.*`, `access.*`, `risk.*`) flow through the `genid-events` JetStream stream. Publishers: identity-service (via transactional outbox relay) and the ingestion edge. Consumers: risk processor, audit ledger, future webhook dispatcher.
2. **External brokers (Kafka, Azure Service Bus, RabbitMQ) are edge adapters only.** If a customer or partner (e.g., ObserveID) publishes to Azure Service Bus or Kafka, a thin adapter subscribes there and republishes canonical events onto NATS. The core never depends on an external broker.
3. **Neo4j is the relationship brain, never a message bus.** It stores identity state, edges, and risk scores. It does not transport events.
4. **Debezium/Kafka CDC is deferred.** The Postgres outbox → NATS relay already provides the transactional event guarantee. If ObserveID later requires Kafka-native consumption, we add a NATS→Kafka bridge connector without touching the core.

## Why NATS JetStream over Kafka (internal)

| Concern | Kafka | NATS JetStream |
|---|---|---|
| Dev footprint | ~1GB+ JVM, KRaft config | ~15MB single binary |
| Ops burden | Partitions, ISR, rebalancing | Stream config, done |
| Queue-group consumption | Consumer groups | Native queue subscriptions |
| Replay/durability | Yes | Yes (file storage, acks, durable consumers) |
| Fit for "modular monolith, 3 binaries" | Poor | Excellent |
| Customer-facing ecosystem | Huge | Small — hence edge bridges |

The decision optimizes for our stakeholder's stated constraint: open-source leverage, low cost, no ocean-boiling. We adopt Kafka only where the outside world forces us to — at the adapter boundary.

## Consequences

- `internal/eventbus/nats.go` is the only bus implementation the core imports.
- New external source = new adapter in `internal/eventing/sources/`, never a new core dependency.
- Documentation must show NATS (not Kafka) in the running-stack tables. STATUS.md corrected same day as this ADR.
