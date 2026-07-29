# GenID V1 — Documentation

> **Version 1 (GenID)** — The current working IAM platform.
> 
> **Version 2 (Fortune Identity Cloud)** — See `v2-archive/` for planning docs.

---

## V1 Documentation

| Document | Purpose |
|----------|---------|
| [STATUS.md](./STATUS.md) | Current system status, API endpoints, architecture |
| [../README.md](../README.md) | Project overview, quick start, features |
| [../SECURITY.md](../SECURITY.md) | Security policy |
| [../AGENTS.md](../AGENTS.md) | Graphify knowledge graph instructions |

---

## V1 Status

- **Backend:** Go 1.25, 33K+ lines, 40+ API endpoints
- **Frontend:** Next.js 14, 15 pages
- **Database:** PostgreSQL 16 (20 tables), Neo4j 5 (8 node labels)
- **Workflows:** 7 Temporal workflows
- **Connectors:** 6 built (Entra ID, AD, LDAP, SCIM, Okta, Generic)
- **Tests:** 112 passed

---

**Last Updated:** 2026-07-22  
**Status:** V1 Complete — Ready for production use
