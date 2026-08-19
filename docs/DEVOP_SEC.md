# DevOpSec — Development, Operations & Security Process

This document is the operating procedure for GenID: how code gets built, scanned,
deployed, and how secrets/credentials are handled. Read it before adding a dependency,
a Docker image, a secret, or a workflow.

## 1. Repository facts

- Repo: `github.com/ShoaibsProjects/GenID` (**PUBLIC** — assume anything you commit is visible).
- Default branch: `main`. Branch protection is **enforced** (see §6).
- Container registry: `ghcr.io` (GitHub Container Registry), images:
  - `ghcr.io/ShoaibsProjects/GenID/identity-service`
  - `ghcr.io/ShoaibsProjects/GenID/event-processor`
  - `ghcr.io/ShoaibsProjects/GenID/frontend`

## 2. Secrets handling (rules)

1. **Never commit secrets.** No passwords, tokens, API keys, or SMTP credentials in code,
   compose files, or configs. The repo is public and gitleaks scans every push.
2. **Local dev secrets live in `infrastructure/.env`** (gitignored). Template:
   `infrastructure/.env.example` (placeholders only, committed).
3. **Production secrets live in CI/CD secrets** (`gh-action`/repo secrets) or the platform
   secret store — never in the repo.
4. **Compose files use `${VAR:-default}`** so `docker compose up` works with dev defaults,
   and real values are injected via `.env` or the environment.
5. **If a secret leaks:** rotate it immediately (do not delete-and-push — rotation is the fix).
   If it was committed, rewrite history only with explicit approval (see §8).
6. **Gmail app passwords** are `16 chars in 4x4 groups` (e.g. `xxxx xxxx xxxx xxxx`).
   A custom gitleaks rule catches these in `SMTP_PASS`/`password` assignments — regression guard.

## 3. Dependency policy

- **Go (backend):** `go get` only pinned minor versions. Keep `govulncheck` clean
  (no reachable module vulns). Build with the Go toolchain pinned in CI (`GO_VERSION=1.26`).
  Stdlib vulns are resolved by keeping the toolchain ≥ the patched release (1.26.6+).
- **Node (frontend):** `npm ci` from the lockfile; `npm audit` must be clean at
  `--audit-level=high`. Review the churn of any `next` major bump before merging.
- **Protobuf:** generated code (`backend/pkg/proto/**`) is **committed**.
  Regenerate locally with `make proto` when `.proto` files change — CI no longer runs `protoc`
  (removes `@latest` drift from builds).

## 4. Container image policy

- **Base images are pinned** (no `latest`): `golang:1.26.6-alpine`, `alpine:3.21`,
  `node:22-alpine`, `nginx:1.27-alpine`. Update deliberately, not casually.
- **Non-root runtime:** services run as `genid` (uid 1001) / `nginx` (uid 101);
  nginx writes pid/temp under `/tmp`.
- **OCI labels** (source, title, description, version) are set from the `VERSION` build-arg.
- **Healthchecks** are defined on every image/service.
- Every image is scanned with **Trivy (HIGH/CRITICAL, ignore-unfixed)** before it is allowed
  to push in `docker-publish.yml`.
- Images are built with **BuildKit provenance + SBOM**; tagged release images are signed
  with **cosign (keyless)**.

## 5. CI gates (`.github/workflows/ci.yml`)

| Gate | Where | Failures |
|------|-------|----------|
| Secret scan | `secrets` job | gitleaks (full history + working tree) |
| Go vet / build / race tests | `backend` job | any failure |
| Go vuln scan | `backend` job | `govulncheck` reachable vulns |
| npm audit | `frontend` job | HIGH+ vulnerabilities |
| Lint / unit tests | `frontend` job | 0 errors, all tests |
| Next.js build | `frontend` job | static export failure |

`docker-publish.yml` runs **Trivy scans after build and before push** — a HIGH/CRITICAL
finding blocks publication.

## 6. Branch protection (main)

Enforced via GitHub:
- **Require a pull request** before merging (at least 1 approval).
- **Require status checks** to pass: `Secret Scan (gitleaks)`,
  `Go Backend (vet, test, vuln scan)`, `Next.js Frontend (lint, test, audit, build)`.
- **Require signed commits** and **linear history**; dismiss stale reviews.
- No one (including admins) force-pushes to `main`.

## 7. Local hygiene

- `git status` must be clean of junk; build artifacts are gitignored.
- `.opencode/` (personal AI config) and `graphify-out/` are untracked.
- Run `gitleaks detect --source .` locally before pushing (pre-commit hook under
  `.githooks/pre-commit` runs it too).

## 8. Escalation / exceptions

- **History rewrite** (e.g. `git filter-repo` to purge a leak) requires explicit user
  approval — it changes every downstream SHA.
- **Dependency bump that fails the gates** is resolved by upgrading, not by disabling a gate.
- To add an allowlist entry in `.gitleaks.toml`, show the exact finding and why it is a
  true false positive (docs prose / test fixture / dev-only value).

## 9. Runbook (common ops)

```bash
# Local stack with dev secrets
cp infrastructure/.env.example infrastructure/.env   # then edit
docker compose -f infrastructure/docker-compose.yml up --build

# Security scans
gitleaks detect --source .
cd backend  && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
cd frontend && npm audit --audit-level=high

# Image build + scan (local, no push)
docker build --build-arg VERSION=dev -f docker/identity-service.Dockerfile -t genid/identity-service:dev .
docker scan genid/identity-service:dev   # or: trivy image genid/identity-service:dev
```