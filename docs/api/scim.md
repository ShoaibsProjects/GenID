# SCIM 2.0 Gateway

GenID implements [SCIM 2.0 (RFC 7644)](https://datatracker.ietf.org/doc/html/rfc7644) user provisioning at `/scim/v2`. This is the standard integration point for HR systems, identity providers (Entra ID, Okta), and provisioning tools.

## Discovery

| Method | Path | Description |
|--------|------|-------------|
| GET | `/scim/v2/ServiceProviderConfig` | Service provider configuration (supports patch, filter, no bulk/sort/etag; auth via bearer token + API key) |
| GET | `/scim/v2/ResourceTypes` | Resource types (User) |
| GET | `/scim/v2/Schemas` | Supported schemas |

## Users

| Method | Path | Description |
|--------|------|-------------|
| GET | `/scim/v2/Users` | List users (paged via `count`/`startIndex`) |
| POST | `/scim/v2/Users` | Create user → `201` with `id` |
| GET | `/scim/v2/Users/{id}` | Get user |
| PUT | `/scim/v2/Users/{id}` | Full update (replacement) |
| PATCH | `/scim/v2/Users/{id}` | Partial update (currently aliases PUT — full replacement) |
| DELETE | `/scim/v2/Users/{id}` | **Deprovision** → `202 {"status":"offboarding","workflow_id":"offboard-<id>"}`; starts `OffboardIdentityWorkflow` (risk-tiered revocation, NHI cascade, CAEP broadcast) |

## User object (create)

```json
{
  "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
  "userName": "alice@example.com",
  "displayName": "Alice Example",
  "emails": [
    { "value": "alice@example.com", "primary": true }
  ],
  "active": true,
  "groups": []
}
```

## Response shapes (RFC 7644)

List response:

```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
  "totalResults": 42,
  "Resources": []
}
```

User response:

```json
{
  "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
  "id": "<uuid>",
  "meta": {
    "resourceType": "User",
    "lastModified": "2026-01-01T00:00:00Z"
  }
}
```

## Behavior notes

- Create is **synchronous**: identity is written to Postgres inside a transaction and an `identity.created` outbox event is published atomically; Neo4j sync happens asynchronously (~100ms). Returns the new id immediately.
- Delete is **asynchronous**: it starts the Temporal offboarding workflow and returns `202` with the workflow id.
- Authentication is via bearer token (JWT or API key) — SCIM routes are not master-key guarded despite a `scim_write` guard existing in the operation enum.
- Provisioning latency is sub-100ms for create (measured ~34–76 ms).

## Related

- [Identity API](overview.md#identities)
- [Temporal offboarding workflow](../workflows/temporal.md)
