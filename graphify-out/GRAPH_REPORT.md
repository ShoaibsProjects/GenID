# Graph Report - .  (2026-07-20)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 1079 nodes · 2406 edges · 63 communities (41 shown, 22 thin omitted)
- Extraction: 96% EXTRACTED · 4% INFERRED · 0% AMBIGUOUS · INFERRED: 108 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `13ca9cc4`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Community 0
- Community 1
- Community 2
- Community 3
- Community 4
- Community 5
- Community 6
- Community 7
- Community 8
- Community 9
- Community 10
- Community 11
- Community 12
- Community 13
- Community 14
- Community 15
- Community 16
- Community 17
- Community 18
- Community 19
- Community 20
- Community 21
- Community 22
- Community 23
- Community 24
- Community 25
- Community 26
- Community 27
- Community 28
- Community 29
- Community 30
- Community 31
- Community 32
- Community 33
- Community 34
- Community 35
- Community 36
- Community 37
- Community 38
- Community 39
- Community 40
- Community 41
- Community 42
- Community 43
- Community 44
- Community 45
- Community 46
- Community 47
- Community 48
- Community 49
- Community 50
- Community 51
- Community 55
- Community 56
- Community 57
- Community 62

## God Nodes (most connected - your core abstractions)
1. `IdentityService` - 87 edges
2. `respondJSON()` - 67 edges
3. `respondError()` - 50 edges
4. `apiRequest()` - 40 edges
5. `EntraConnector` - 38 edges
6. `Manager` - 35 edges
7. `LDAPConnector` - 33 edges
8. `SCIMConnector` - 32 edges
9. `CSVConnector` - 31 edges
10. `ActivityService` - 29 edges

## Surprising Connections (you probably didn't know these)
- `NewIdentityService()` --calls--> `NewStore()`  [INFERRED]
  backend/internal/service/identity_service.go → backend/internal/audit/audit.go
- `NewConnector()` --calls--> `NewCSVConnector()`  [INFERRED]
  backend/internal/connector/manager.go → backend/internal/connector/csv.go
- `NewConnector()` --calls--> `NewEntraConnector()`  [INFERRED]
  backend/internal/connector/manager.go → backend/internal/connector/entra.go
- `NewIdentityService()` --calls--> `NewManager()`  [INFERRED]
  backend/internal/service/identity_service.go → backend/internal/connector/manager.go
- `NewConnector()` --calls--> `NewSCIMConnector()`  [INFERRED]
  backend/internal/connector/manager.go → backend/internal/connector/scim.go

## Import Cycles
- None detected.

## Communities (63 total, 22 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.10
Nodes (21): evaluateCedarPolicy(), getRecordString(), getRecordStrings(), getRecordVal(), Client, ConnectorConfig, ConnectorGroup, ConnectorUser (+13 more)

### Community 1 - "Community 1"
Cohesion: 0.05
Nodes (45): actions, DURATION_PRESETS, Badge(), BadgeProps, BadgeVariant, colors, Button(), ButtonProps (+37 more)

### Community 2 - "Community 2"
Cohesion: 0.07
Nodes (34): ActivityService, AnomalyResult, AssignRoleParams, AuditTrailParams, AuditTrailResult, CAEPEventParams, CreateIdentityParams, CreateIdentityResult (+26 more)

### Community 3 - "Community 3"
Cohesion: 0.09
Nodes (31): NewLDAPConnector(), Connector, ConnectorConfig, ConnectorGroup, ConnectorStatus, ConnectorType, ConnectorUser, Context (+23 more)

### Community 4 - "Community 4"
Cohesion: 0.10
Nodes (38): Handler, RWMutex, NewAPIKeyAuth(), Request, ResponseWriter, T, okHandler(), TestAPIKeyAuth_AcceptsBearerToken() (+30 more)

### Community 5 - "Community 5"
Cohesion: 0.10
Nodes (18): Entry, RawMessage, getAttr(), getAttrs(), ConnectorConfig, ConnectorGroup, ConnectorStatus, ConnectorType (+10 more)

### Community 6 - "Community 6"
Cohesion: 0.10
Nodes (15): extractDeltaToken(), generateTempPassword(), Client, ConnectorConfig, ConnectorGroup, ConnectorStatus, ConnectorType, ConnectorUser (+7 more)

### Community 7 - "Community 7"
Cohesion: 0.10
Nodes (36): Filter, Level, responseWriter, Store, StoreStats, Handler, RWMutex, Time (+28 more)

### Community 8 - "Community 8"
Cohesion: 0.12
Nodes (11): Client, ConnectorConfig, ConnectorGroup, ConnectorStatus, ConnectorType, ConnectorUser, Context, RawMessage (+3 more)

### Community 9 - "Community 9"
Cohesion: 0.08
Nodes (38): AgentsResponse, ConnectorConfig, getApiBase(), GQLAuditEntry, gqlConnectConnector(), GQLConnector, GQLConnectorGroup, GQLConnectorHealth (+30 more)

### Community 10 - "Community 10"
Cohesion: 0.11
Nodes (9): ConnectorConfig, ConnectorGroup, ConnectorStatus, ConnectorType, ConnectorUser, Context, NewCSVConnector(), sanitizeCSVPath() (+1 more)

### Community 11 - "Community 11"
Cohesion: 0.13
Nodes (36): agent_cards, audit_log, caep_events, cedar_policies, certification_campaigns, certification_entries, connector_entitlements, connector_groups (+28 more)

### Community 12 - "Community 12"
Cohesion: 0.12
Nodes (22): deriveKey(), generateSecretID(), Context, RWMutex, Time, NewVault(), T, TestConcurrency() (+14 more)

### Community 13 - "Community 13"
Cohesion: 0.09
Nodes (24): ConnectorStatus, ConnectorType, Time, ifaceStr(), AuditEntry, Connector, ConnectorGroup, ConnectorHealth (+16 more)

### Community 14 - "Community 14"
Cohesion: 0.08
Nodes (25): apiRequest(), assignRole(), broadcastCAEP(), copilotQuery(), createGroup(), createIdentity(), deleteGroup(), deleteIdentity() (+17 more)

### Community 15 - "Community 15"
Cohesion: 0.26
Nodes (12): Connector, ConnectorGroup, ConnectorUser, Context, RWMutex, Time, NewProvisioningEngine(), LCMRequest (+4 more)

### Community 16 - "Community 16"
Cohesion: 0.08
Nodes (17): Connector, CONNECTOR_TYPE_FIELDS, CONNECTOR_TYPES, ConnectorStats, HealthReport, STATUS_COLORS, SyncStats, TabKey (+9 more)

### Community 17 - "Community 17"
Cohesion: 0.11
Nodes (5): ConnectorStatus, ConnectorType, IdentityStatus, IdentityType, Writer

### Community 18 - "Community 18"
Cohesion: 0.20
Nodes (9): CopilotPipeline, CopilotQuery, CopilotResponse, containsAny(), Context, DriverWithContext, Record, NewCopilotPipeline() (+1 more)

### Community 19 - "Community 19"
Cohesion: 0.11
Nodes (19): autoprefixer, eslint, eslint-config-next, devDependencies, autoprefixer, eslint, eslint-config-next, jest (+11 more)

### Community 20 - "Community 20"
Cohesion: 0.22
Nodes (18): AgentAnomalyDetectionWorkflow(), CascadeRevokeWorkflow(), DetectSoDViolationsWorkflow(), executeRevocationWithRetry(), Context, GrantAccessWorkflow(), JustInTimeAccessWorkflow(), OffboardIdentityWorkflow() (+10 more)

### Community 21 - "Community 21"
Cohesion: 0.11
Nodes (19): clsx, dependencies, clsx, @headlessui/react, @heroicons/react, lucide-react, next, react (+11 more)

### Community 22 - "Community 22"
Cohesion: 0.14
Nodes (13): AuditPage(), formatTimestamp(), LEVEL_BADGE, LEVEL_BG, LEVEL_COLORS, LogEntry, LogStats, METHOD_COLORS (+5 more)

### Community 24 - "Community 24"
Cohesion: 0.23
Nodes (15): Time, AgentCard, AuditRecord, CAEPEvent, CedarPolicy, DelegationChain, Entitlement, EntityType (+7 more)

### Community 25 - "Community 25"
Cohesion: 0.33
Nodes (5): configToGQLConnector(), Connector, ConnectorConfig, Context, mutationResolver

### Community 26 - "Community 26"
Cohesion: 0.26
Nodes (5): ConnectorGroup, ConnectorUser, identityToGQL(), queryResolver, Identity

### Community 27 - "Community 27"
Cohesion: 0.17
Nodes (11): getRiskStyle(), IdentitiesPage(), Identity, PAGE_SIZES, RISK_CLASSES, SOURCES, STATUS_COLORS, STATUSES (+3 more)

### Community 28 - "Community 28"
Cohesion: 0.18
Nodes (10): name, private, scripts, build, dev, lint, start, test (+2 more)

### Community 29 - "Community 29"
Cohesion: 0.20
Nodes (5): AgentsPage(), Agent, fetchAgents(), fetchHealth(), fetchIdentities()

### Community 30 - "Community 30"
Cohesion: 0.36
Nodes (4): Request, NewWorkflowGuard(), OperationType, WorkflowGuard

### Community 31 - "Community 31"
Cohesion: 0.33
Nodes (3): Connector, DeltaNotSupportedError, NotSupportedError

### Community 32 - "Community 32"
Cohesion: 0.33
Nodes (3): Resolver, MutationResolver, QueryResolver

### Community 33 - "Community 33"
Cohesion: 0.40
Nodes (4): extends, rules, react/no-unescaped-entities, next/core-web-vitals

### Community 34 - "Community 34"
Cohesion: 0.60
Nodes (3): SettingsPage(), getApiUrl(), setApiUrl()

## Knowledge Gaps
- **124 isolated node(s):** `github.com/observeid/identity-platform`, `Connector`, `EntitlementType`, `ResourceType`, `ProvisioningRequest` (+119 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **22 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `IdentityService` connect `Community 0` to `Community 32`, `Community 3`, `Community 7`, `Community 12`, `Community 15`?**
  _High betweenness centrality (0.226) - this node is a cross-community bridge._
- **Why does `Store` connect `Community 7` to `Community 0`, `Community 5`?**
  _High betweenness centrality (0.096) - this node is a cross-community bridge._
- **Why does `LoggingMiddleware()` connect `Community 7` to `Community 4`?**
  _High betweenness centrality (0.082) - this node is a cross-community bridge._
- **What connects `github.com/observeid/identity-platform`, `Connector`, `EntitlementType` to the rest of the system?**
  _124 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.09736842105263158 - nodes in this community are weakly interconnected._
- **Should `Community 1` be split into smaller, more focused modules?**
  _Cohesion score 0.05092276144907724 - nodes in this community are weakly interconnected._
- **Should `Community 2` be split into smaller, more focused modules?**
  _Cohesion score 0.07130333138515488 - nodes in this community are weakly interconnected._