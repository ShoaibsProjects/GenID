-- ─── Migration 002: Identity Automation Workflows ──────────────────
-- Part of IDENTITY_AUTOMATION.md Phase 2 plan.
--
-- Adds the three core tables that every workflow writes through:
--   - workflow_requests:   the request itself (one row per workflow invocation)
--   - workflow_approvals:  per-level approval decisions (signals from humans)
--   - workflow_audit:      immutable step-by-step trail (eligibility, policy, exec, notify)
--
-- IDEMPOTENT: safe to run repeatedly (IF NOT EXISTS / DO blocks).
-- ADDITIVE: no existing column/table is dropped or renamed.

-- ─── 1. workflow_requests ────────────────────────────────────────
-- One row per workflow invocation. Lifecycle:
--   pending → eligible → approved → executed → (completed | failed | cancelled)
--   pending → eligible → denied → (cancelled | completed after review)
--
-- payload is action-specific (target identity, role, duration, justification, ...).
-- The Temporal workflow_id lives here so we can signal/cancel from the API.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'workflow_requests'
    ) THEN
        CREATE TABLE workflow_requests (
            id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            tenant_id           UUID NOT NULL,
            type                VARCHAR(80) NOT NULL,
            -- e.g. access.request.jit, access.request.firecall, access.request.permanent,
            --      account.deactivate, account.purge, role.create, cert.campaign.generate
            status              VARCHAR(30) NOT NULL DEFAULT 'pending',
            requester_id        UUID,
            target_id           UUID,
            payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
            -- idempotency_key is a client-supplied dedup key (e.g. UI UUID per click).
            -- UNIQUE per tenant+type to make double-click safe.
            idempotency_key     TEXT,
            temporal_workflow_id TEXT,
            temporal_run_id     TEXT,
            created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            expires_at          TIMESTAMPTZ,    -- auto-deny if no decision by then
            completed_at        TIMESTAMPTZ,
            failure_reason      TEXT
        );

        CREATE UNIQUE INDEX IF NOT EXISTS workflow_requests_idem_uniq
            ON workflow_requests (tenant_id, type, COALESCE(idempotency_key, ''));
        CREATE INDEX IF NOT EXISTS workflow_requests_status_idx
            ON workflow_requests (status) WHERE status IN ('pending', 'eligible', 'approved');
        CREATE INDEX IF NOT EXISTS workflow_requests_requester_idx
            ON workflow_requests (requester_id, created_at DESC);
        CREATE INDEX IF NOT EXISTS workflow_requests_target_idx
            ON workflow_requests (target_id, created_at DESC);
        CREATE INDEX IF NOT EXISTS workflow_requests_type_idx
            ON workflow_requests (type, created_at DESC);
    END IF;
END $$;

-- ─── 2. workflow_approvals ────────────────────────────────────────
-- One row per approval level. Sequential numbering (1, 2, 3, ...).
-- A signal from the human updates status + comment + decided_at.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'workflow_approvals'
    ) THEN
        CREATE TABLE workflow_approvals (
            id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            request_id      UUID NOT NULL REFERENCES workflow_requests(id) ON DELETE CASCADE,
            level           INT NOT NULL,
            approver_id     UUID,
            approver_email  TEXT,
            approver_role   VARCHAR(80),    -- manager | resource_owner | security | ciso_delegate | auto
            status          VARCHAR(30) NOT NULL DEFAULT 'pending',
            -- pending | approved | denied | skipped | escalated | expired
            comment         TEXT,
            decided_at      TIMESTAMPTZ,
            due_at          TIMESTAMPTZ,
            created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            UNIQUE (request_id, level)
        );

        CREATE INDEX IF NOT EXISTS workflow_approvals_pending_idx
            ON workflow_approvals (status, due_at) WHERE status = 'pending';
        CREATE INDEX IF NOT EXISTS workflow_approvals_approver_idx
            ON workflow_approvals (approver_id, status);
    END IF;
END $$;

-- ─── 3. workflow_audit ────────────────────────────────────────────
-- Immutable step log. Every workflow writes one row per activity it runs:
--   - workflow.requested       (creation)
--   - eligibility.check        (can this user request this?)
--   - policy.evaluate          (SoD, risk, geo, time)
--   - approval.routed          (who must approve)
--   - approval.received        (decision from human)
--   - activity.started         (Temporal activity begin)
--   - activity.completed       (Temporal activity end)
--   - activity.failed          (compensation triggered)
--   - workflow.approved        (final state)
--   - workflow.denied          (final state)
--   - workflow.executed        (final state)
--   - workflow.failed          (final state)
--   - workflow.cancelled       (final state)
--   - notify.sent              (channel + recipient)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'workflow_audit'
    ) THEN
        CREATE TABLE workflow_audit (
            id              BIGSERIAL PRIMARY KEY,
            request_id      UUID REFERENCES workflow_requests(id) ON DELETE CASCADE,
            step            VARCHAR(80) NOT NULL,
            actor           TEXT NOT NULL DEFAULT 'system',
            -- 'system' | 'user:<id>' | 'temporal:<workflow_id>' | 'webhook:<source>'
            details         JSONB NOT NULL DEFAULT '{}'::jsonb,
            ts              TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );

        CREATE INDEX IF NOT EXISTS workflow_audit_request_idx
            ON workflow_audit (request_id, ts);
        CREATE INDEX IF NOT EXISTS workflow_audit_step_idx
            ON workflow_audit (step, ts DESC);
    END IF;
END $$;
