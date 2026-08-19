# Neo4j Graph Model

Neo4j stores the **relationship graph** (who has access to what, via which path), **risk scores**, **sessions**, and **reviews**. Source: `infrastructure/neo4j/init.cypher` + Go Cypher in the backend.

## Constraints & indexes

### Uniqueness
| Label | Property |
|-------|----------|
| Identity | `uuid` |
| Identity | `(tenant_id, email)` |
| NonHumanIdentity | `uuid` |
| Entitlement | `uuid` |
| Resource | `uuid` |
| Role | `uuid` |
| Session | `session_id` |
| Policy | `uuid` |

### Indexes
Identity: `(tenant_id, status)`, `email`, `department`; NonHumanIdentity: `owner_id`, `status`, `is_governed`; Entitlement: `app_name`, `is_toxic`, `risk_classification`; Resource: `type`, `criticality`, `data_classification`; Role: `name`; Session: `is_active`, `identity_uuid`; **full-text** `identity_search [display_name,email]`, `nhi_search [name]`; relationship indexes on `HAS_ROLE (expires_at, is_active)` and `DELEGATED_FROM (max_depth_remaining)`.

## Nodes

### Identity
`uuid, tenant_id, type, status, email, display_name, first_name, last_name, department, title, employee_id, manager_id, source, phone, risk_score, risk_factors, assurance_level, employment_type, created_at, updated_at, last_reviewed_at, last_accessed_at`

### NonHumanIdentity
`uuid, tenant_id, name, type, status, agent_card_id, protocols, owner_id, owner_name, team_id, capabilities, deployment_environment, framework, is_governed, risk_score, risk_factors, created_at, revoked_at`

### Role
`uuid, tenant_id, name, description, role_type, is_auto_assigned, approval_required, is_active, created_at`

### Entitlement
`id, app_name, permission_level, entitlement_type, risk_classification, is_toxic, is_rubberband, expires_at, condition, status, revoked_at, revoked_by`

### Resource
`uuid, name, type, criticality, data_classification, owner_team, connection_type`

### Session
`uuid, status, source, ip_address, created_at, last_activity, terminated_at, termination_reason`

### Review
`uuid, trigger_type, risk_score, risk_band, description, status, created_at, due_date, decision, reviewer, completed_at`

### Persona
`id, created_at, email, employee_id`

## Risk-score properties (dynamic)

Written by the event processor / combined risk:

`risk_score, risk_static, risk_dynamic, risk_peer, risk_band, risk_factors, risk_calculated_at, risk_last_event, risk_last_source, risk_last_severity, risk_event_count, risk_last_updated`

## Relationships

| Relationship | Source → Target | Key properties |
|--------------|-----------------|----------------|
| `HAS_ROLE` | Identity/NHI → Role | `assigned_at, assigned_by, source, is_active, granted_at, granted_by, reason, expires_at` |
| `GRANTS` | Role → Entitlement | `granted_at, condition, expires_at` |
| `ACCESSES` | Entitlement → Resource | `access_type` |
| `HAS_ENTITLEMENT` | Identity → Entitlement | *(traversed, not created)* |
| `DIRECTLY_OWNS` | Identity → Entitlement | *(traversed)* |
| `HAS_DIRECT_ACCESS` | Identity/NHI → Resource | `granted_at, granted_by, reason, source` |
| `HAS_TEMPORARY_ACCESS` | Identity/NHI → Resource | `granted_at, expires_at (epoch ms), granted_by` |
| `HAS_SESSION` | Identity/NHI → Session | — |
| `HAS_REVIEW` | Identity/NHI → Review | — |
| `OWNED_BY` | NonHumanIdentity → Identity | `ownership_type: 'primary'` |
| `DELEGATED_FROM` | NonHumanIdentity(child) → NonHumanIdentity(parent) | `delegated_at, scope_narrowing, max_depth_remaining` |
| `MANAGES` | Identity → Identity | — |
| `RESOLVES_TO` | Identity → Persona | — |
| `CONFLICTS_WITH` | Entitlement → Entitlement | `conflict_type, severity, detected_at` |
| `INCOMPATIBLE_WITH` | Role → Role | *(SoD, traversed)* |

## Canonical access path

The traversal used by risk scoring, access checks, and the copilot:

```
(i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(:Entitlement)-[:ACCESSES]->(:Resource)
```

plus direct paths: `HAS_ENTITLEMENT`, `HAS_DIRECT_ACCESS`, `HAS_TEMPORARY_ACCESS`.

## PG ↔ Neo4j sync

- **Transactional outbox** (`outbox_events`): identity/role/entitlement events applied to Neo4j by the outbox processor, then republished to NATS.
- **Direct dual-write**: some handlers write both stores (identity create via GraphQL, role assignment, NHI registration, delegation, persona stitching).
- **NATS event processor**: risk scores/sessions/reviews written to Neo4j from `genid-events.>`.

## Demo seed data

The risk demo seeds a small graph (identities `demo-alice`/`demo-bob`/`demo-carol`/`demo-dave`, roles like ERP Admin / SAP User / Jira User, entitlements e1–e4, resources r1–r4, with `has_role`/`has_entitlement`/`has_direct_access`/`accesses` edges). `demo-dave` is deliberately over-privileged (ERP Admin + Prod DB) to demonstrate static + peer risk without any events.

## Related

- [PostgreSQL schema](postgres.md)
- [Risk engine](../architecture/risk-engine.md)
- [Events catalog](../events/catalog.md)
