# ObserveID Identity Fabric — System Status

> **Built:** July 2026 | **Go 1.25** | **Next.js 14** | **Cloudflare Free Tier**
> **Architecture:** Event-Driven | AI-Native | Zero-Trust IAM/IGA Platform

---

##  Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Cloudflare Pages                       │
│  (Frontend: observeid-frontend.pages.dev)                │
└──────────────────────┬──────────────────────────────────┘
                       │ API calls (HTTPS)
┌──────────────────────▼──────────────────────────────────┐
│              Cloudflare Tunnel (Quick Tunnel)            │
│         https://[random].trycloudflare.com               │
└──────────────────────┬──────────────────────────────────┘
                       │ localhost:8080
┌──────────────────────▼──────────────────────────────────┐
│              Go Backend (identity-service)               │
│  ┌─────────────┬──────────────┬──────────────┬────────┐ │
│  │ HTTP Router │  Temporal    │  Connector   │ Vault  │ │
│  │ (gorilla)   │  Workflows   │  Framework   │ (AES)  │ │
│  └──────┬──────┴──────┬───────┴──────┬───────┴───┬────┘ │
└─────────┼─────────────┼──────────────┼───────────┼───────┘
          │             │              │           │
     ┌────▼──┐    ┌─────▼─────┐  ┌────▼────┐ ┌───▼────┐
     │Postgres│    │   Neo4j   │  │  Redis  │ │ Kafka  │
     │  :5432 │    │  :7687    │  │ :6379   │ │ :9092  │
     └───────┘    └──────────┘  └─────────┘ └────────┘
          ┌──────────┐   ┌─────────┐   ┌────────┐
          │ Temporal │   │ Qdrant  │   │Grafana │
          │  :7233   │   │ :6333   │   │ :3000  │
          └──────────┘   └─────────┘   └────────┘
```

##  What's Running

| Component | Status | Port |
|-----------|--------|------|
| Go Backend (identity-service) | ✅ Running | `:8080` |
| PostgreSQL 16 | ✅ Docker | `:5432` |
| Neo4j 5 Enterprise | ✅ Docker | `:7474` `:7687` |
| Redis 7 | ✅ Docker | `:6379` |
| Kafka 7.6.1 | ✅ Docker | `:9092` |
| Temporal Server 1.25 | ✅ Docker | `:7233` |
| Qdrant v1.10.1 | ✅ Docker | `:6333` |
| Grafana 11.1 | ✅ Docker | `:3000` |
| OpenTelemetry Collector | ✅ Docker | `:4317` |
| Cloudflare Quick Tunnel | ✅ Running | — |
| Cloudflare Pages | ✅ Deployed | — |

##  Access Points

| Endpoint | URL | Purpose |
|----------|-----|---------|
| **API + Frontend (local)** | `http://localhost:8080` | Full platform |
| **API + Frontend (tunnel)** | see `/tmp/cloudflared.log` | Public access |
| **Frontend (Cloudflare)** | `https://observeid-frontend.pages.dev` | Deployed UI |
| **GitHub Repo** | `https://github.com/ShoaibsProjects/observeid` | Source code |

##  API Endpoints

### Core
| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check |
| GET | `/metrics` | Prometheus metrics |

### SCIM 2.0
| Method | Path | Description |
|--------|------|-------------|
| GET | `/scim/v2/Users` | List users |
| POST | `/scim/v2/Users` | Create user |
| GET/PUT/PATCH/DELETE | `/scim/v2/Users/{id}` | CRUD on user |

### Identity Management
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/identities` | List identities |
| POST | `/api/v1/identities` | Create identity |
| GET | `/api/v1/identities/{id}` | Get identity details |
| PATCH | `/api/v1/identities/{id}` | Update identity |
| DELETE | `/api/v1/identities/{id}` | Delete identity |
| GET | `/api/v1/identities/{id}/entitlements` | Get entitlements |
| GET | `/api/v1/identities/{id}/blast-radius` | Get blast radius |

### Agent / NHI Management
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/agents` | List agents |
| POST | `/api/v1/agents` | Register agent |
| GET | `/api/v1/agents/{id}` | Get agent details |
| POST | `/api/v1/agents/{id}/kill-switch` | Emergency agent kill |
| POST | `/api/v1/agents/{id}/delegate` | Delegate agent authority |
| GET | `/api/v1/agents/{id}/card` | Get Agent Card (A2A/MCP) |

### Access Control
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/access/check` | Check access permission |
| POST | `/api/v1/access/grant` | Grant access (Temporal workflow) |
| POST | `/api/v1/access/revoke` | Revoke access (Temporal workflow) |

### Role / Group Management (RBAC)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/groups` | List groups/roles |
| POST | `/api/v1/groups` | Create group/role |
| DELETE | `/api/v1/groups/{id}` | Delete group/role |
| POST | `/api/v1/roles/assign` | Assign role to identity |
| POST | `/api/v1/roles/remove` | Remove role from identity |

### Connector Framework
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/connectors` | List connectors |
| POST | `/api/v1/connectors` | Register connector |
| POST | `/api/v1/connectors/test` | Test connection |
| GET | `/api/v1/connectors/{id}` | Get connector details |
| DELETE | `/api/v1/connectors/{id}` | Delete connector |
| POST | `/api/v1/connectors/{id}/connect` | Connect connector |
| POST | `/api/v1/connectors/{id}/disconnect` | Disconnect connector |
| POST | `/api/v1/connectors/{id}/sync` | Sync users from connector |

### IAM Lifecycle Management (LCM)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/lcm` | Execute lifecycle action |
| GET | `/api/v1/lcm/history` | Provisioning history |

### Credential Vault
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/vault/secrets` | List stored secrets |
| POST | `/api/v1/vault/secrets` | Store secret (AES-256-GCM encrypted) |
| GET | `/api/v1/vault/secrets/{id}` | Retrieve secret |
| DELETE | `/api/v1/vault/secrets/{id}` | Delete secret |

### CAEP
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/caep/events` | List CAEP events |
| POST | `/api/v1/caep/broadcast` | Broadcast CAEP event |

### AI Copilot
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/copilot/query` | AI identity queries |

##  Connectors (Built)

| Connector | Type | Protocol | Auth |
|-----------|------|----------|------|
| Microsoft Entra ID | `entra_id` | Microsoft Graph API | OAuth2 (client_credentials) |
| Active Directory | `active_directory` | LDAP/S | NTLM/Kerberos/Basic |
| LDAP | `ldap` | LDAP/S | Simple/Bind |
| Universal SCIM | `scim` | SCIM 2.0 | OAuth2/Basic/Bearer/API Key |
| Okta | `okta` | SCIM 2.0 | OAuth2 |
| Generic | `generic` | SCIM 2.0 | Configurable |

##  Temporal Workflows

| Workflow | Queue | Description |
|----------|-------|-------------|
| `OnboardIdentityWorkflow` | provisioning | Create identity, assign roles |
| `OffboardIdentityWorkflow` | critical_offboarding | Full offboarding with cascade |
| `GrantAccessWorkflow` | provisioning | Grant with optional approval |
| `RevokeAccessWorkflow` | critical_offboarding | Emergency revocation |
| `JustInTimeAccessWorkflow` | provisioning | Temp access with timer |
| `AgentAnomalyDetectionWorkflow` | analysis | Cron: anomaly scanning |
| `DetectSoDViolationsWorkflow` | analysis | Cron: SoD violation scanning |

##  Database Schema (PostgreSQL + Neo4j)

**PostgreSQL (20 tables):**
`tenants`, `identities`, `non_human_identities`, `roles`, `entitlements`, `resources`, `identity_roles`, `role_entitlements`, `direct_entitlements`, `delegation_chains`, `sessions`, `outbox`, `audit_log`, `caep_events`, `cedar_policies`, `agent_cards`, `certification_campaigns`, `certification_entries`, `sod_rules`, `emergency_access`, `connectors`

**Neo4j (8 node labels, 7 relationship types):**
Node: `Identity`, `NonHumanIdentity`, `Role`, `Entitlement`, `Resource`, `Session`, `Policy`
Relationships: `HAS_ROLE`, `GRANTS`, `DIRECTLY_OWNS`, `ACCESSES`, `OWNED_BY`, `DELEGATED_FROM`, `CONFLICTS_WITH`

##  Frontend Pages

| Route | File | Status |
|-------|------|--------|
| `/` | Landing page | ✅ |
| `/dashboard` | Identity fabric dashboard | ✅ Live API |
| `/identities` | Identity list (human + agent tabs) | ✅ Live API |
| `/agents` | AI Agents & NHI | ✅ Live API |
| `/connectors` | Connector management | ✅ CRUD + test |
| `/groups` | Group/RBAC management | ✅ Live API |
| `/access` | Access control checker | ✅ |
| `/vault` | Encrypted credential vault | ✅ AES-256-GCM |
| `/policies` | Policy viewer | 🟡 Placeholder |
| `/audit` | CAEP event log | ✅ Live API |
| `/certifications` | Certification campaigns | 🟡 Placeholder |
| `/sod` | SoD violations | 🟡 Placeholder |

##  Quick Commands

```bash
# Start everything
cd /Users/shoaibakthar/Documents/Shoaib's IAM/observeid
make up                        # Start Docker containers
make proto                     # Generate protobuf
make backend                   # Build Go backend
cd backend && go run ./cmd/identity-service  # Run backend

# Tunnel
cloudflared tunnel --url http://localhost:8080

# Check tunnel URL
grep -o 'https://[a-z-]*\.trycloudflare\.com' /tmp/cloudflared.log | head -1

# Frontend build
cd frontend && npm run build

# Test API
curl http://localhost:8080/health

# Push to GitHub
git add -A && git commit -m "message" && git push origin main
```

##  Security Features

- **Credential Vault**: AES-256-GCM encrypted secret storage
- **Connector Secrets**: Encrypted at rest, decrypted in-memory only
- **Master Key**: Derived via SHA-256 from VAULT_MASTER_KEY env variable
- **Sticky Revocations**: Redis-backed cache prevents re-access after revocation
- **CAEP Events**: Continuous Access Evaluation Protocol broadcasts
- **Cedar Policies**: AWS-style authorization policies (RBAC + ABAC)
- **Audit Trail**: Every action logged with correlation_id and trace_id

##  Future Roadmap

- [ ] Desktop app (Electron) bundling backend + Docker
- [ ] Full policy management UI (Cedar editor)
- [ ] Certification campaign workflows
- [ ] Real LLM integration for AI Copilot (GraphRAG)
- [ ] Custom domain + permanent Cloudflare Tunnel
- [ ] OAuth2/OIDC provider integration
- [ ] SCIM provisioning outbound
- [ ] Passwordless authentication (WebAuthn/FIDO2)
- [ ] Real-time websocket events for UI updates
