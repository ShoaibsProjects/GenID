# GenID Documentation

GenID is a graph-native, policy-as-code Identity & Access Management (IAM/IGA) platform built for the Agentic Era. It secures humans, machines, and autonomous AI agents in real time with event-driven, mathematically certain access control.

## Quick Navigation

| Area | Description |
|------|-------------|
| [Getting Started](getting-started/quickstart.md) | Spin up the full stack locally in minutes |
| [Configuration](getting-started/configuration.md) | Environment variables, ports, credentials, defaults |
| [Architecture](architecture/overview.md) | System design, planes, data flow, components |
| [Risk Engine](architecture/risk-engine.md) | 0-1000 risk scoring deep-dive (the core value prop) |
| [API Reference](api/overview.md) | Complete HTTP API surface, auth, per-module endpoints |
| [Data Model](data-model/postgres.md) | PostgreSQL schema + RLS, and the Neo4j graph model |
| [Workflows](workflows/temporal.md) | Temporal JML, JIT, approvals, revocation, cron jobs |
| [Event Catalog](events/catalog.md) | NATS subjects, payload contracts, risk weights |
| [Demo Guide](guides/demo.md) | Scripted demo for stakeholders |
| [Testing](guides/testing.md) | UI walkthrough and end-to-end verification |
| [Operations](operational/docker.md) | Docker Compose topology, health, troubleshooting |

## Documentation Map

```
docs/
├── README.md                    ← you are here
├── getting-started/
│   ├── quickstart.md            ← local dev bootstrap
│   └── configuration.md         ← env / ports / credentials
├── architecture/
│   ├── overview.md              ← system design & data flow
│   └── risk-engine.md           ← risk scoring internals
├── api/
│   ├── overview.md              ← auth, middleware, full endpoint table
│   ├── scim.md                  ← SCIM 2.0 gateway
│   └── oidc.md                  ← OIDC / OAuth 2.0 provider
├── data-model/
│   ├── postgres.md              ← schema, RLS, outbox
│   └── neo4j.md                 ← graph nodes & relationships
├── workflows/
│   └── temporal.md              ← Temporal workflows & activities
├── events/
│   └── catalog.md               ← NATS event catalog & weights
├── guides/
│   ├── demo.md                  ← stakeholder demo script
│   └── testing.md               ← UI walkthrough + E2E verification
└── operational/
    ├── docker.md                ← compose topology & ops
    └── security.md              ← security model & hardening
```

## Also in this repo

- [`COMPLETE_ARCHITECTURE.md`](architecture/COMPLETE_ARCHITECTURE.md) — full feature map and module-by-module status (comprehensive backlog reference)
- [`TESTING_GUIDE.md`](architecture/TESTING_GUIDE.md) — historical testing walkthrough (superseded by `guides/testing.md`)
- [`../README.md`](../README.md) — project README (stack, quickstart, badges)
- [`../SECURITY.md`](../SECURITY.md) — security policy
- [`../CONTRIBUTING.md`](../CONTRIBUTING.md) — contributing guide
- [`GAP_ANALYSIS_AND_USECASES.md`](GAP_ANALYSIS_AND_USECASES.md) — product gap analysis vs. market
- [`STATUS.md`](STATUS.md) — project status tracker
- [`JOURNEY.md`](JOURNEY.md) — build journey notes
