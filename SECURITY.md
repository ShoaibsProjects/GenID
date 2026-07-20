# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| `main` branch | :white_check_mark: Active development |
| Tagged releases | :white_check_mark: Once released |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Email security reports to the project maintainers. You should receive a response within 48 hours. If the issue is confirmed, we will release a patch as soon as possible.

## Security Architecture

ObserveID implements defense-in-depth for identity data:

- **AES-256-GCM** authenticated encryption for all stored secrets (vault)
- **API key authentication** with configurable key rotation (`API_KEYS` env var)
- **WorkflowGuard** — 12 sensitive operations require master permission
- **HTTP security headers** — nosniff, XFO, HSTS, Permissions-Policy
- **Pre-commit gitleaks hook** — 60+ commits scanned, zero secrets leaked
- **Request validation** — Content-Type enforcement, 10MB body limit, JSON-only
- **Rate limiting** — Per-IP token bucket (100 req/s, burst 200)
- **Cedar policy engine** — Forbid always wins over permit, evaluates at access-check time

## Security Best Practices for Deployers

1. **Set `VAULT_MASTER_KEY`** — a 32-byte hex key (`openssl rand -hex 32`). Do not use the default fallback key.
2. **Set `API_KEYS`** — comma-delimited `name:key` pairs. Without this, auth is disabled.
3. **Set `MASTER_KEY`** — enables WorkflowGuard for sensitive operations.
4. **Enable TLS** — set `TLS_CERT_FILE` and `TLS_KEY_FILE` for production deployments.
5. **Rotate API keys regularly** — the `API_KEYS` env var supports hot-reloading via `SetKeys()`.
6. **Restrict CORS** — set `CORS_ORIGIN` to your frontend domain in production.

## Dependency Security

This project's dependencies are managed via:
- Go: `go.sum` with `go mod verify` checks
- TypeScript: `package-lock.json` with `npm audit`
- Infrastructure: Pinned Docker image tags

Run `gitleaks detect` before committing. The pre-commit hook automates this.
