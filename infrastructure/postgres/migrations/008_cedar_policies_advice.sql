-- 008_cedar_policies_advice.sql
-- Phase 1 (Conditional Access MVP): extend cedar_policies with the
-- advice/priority columns the conditional-access router keys on, and seed
-- the four flagship demo policies for the demo tenant.
--
-- NOTE: the spec referenced "003_cedar_policies.sql" but 003 is taken by
-- the workflow-dedup fix; these columns land here instead. is_active
-- already exists on the table, so it is not re-added.
--
-- The Cedar language has no `advice` keyword; the flag policies use the
-- equivalent standard annotation syntax @advice("...") which cedar-go
-- exposes via Policy.Annotations(). The engine reads the annotation from
-- the matched policy to route Allow/StepUp/Deny + duration.

BEGIN;

ALTER TABLE cedar_policies
    ADD COLUMN IF NOT EXISTS advice  VARCHAR(50),
    ADD COLUMN IF NOT EXISTS priority INT DEFAULT 100;

-- Policy 1: IT Admin + Corporate + Business Hours + Managed + Low Risk → Auto-Approve 2h JIT
INSERT INTO cedar_policies (tenant_id, policy_id, effect, policy_source, cedar_text, is_active, version, advice, priority) VALUES
('00000000-0000-0000-0000-000000000001', 'pol_auto_approve_2h', 'permit', '', $cedar$
@advice("auto_approve_2h")
permit(
    principal,
    action == Action::"grant",
    resource
)
when {
    context.role == "it-admin" &&
    context.network_zone == "corporate" &&
    context.time_of_day == "business_hours" &&
    context.device_trust == "managed" &&
    context.risk_score < 500
};
$cedar$, TRUE, 1, 'auto_approve_2h', 10),
-- Policy 2: Public Network OR Unmanaged Device → Step-Up Approval
('00000000-0000-0000-0000-000000000001', 'pol_step_up_approval', 'forbid', '', $cedar$
@advice("step_up_approval")
forbid(
    principal,
    action == Action::"grant",
    resource
)
when {
    context.network_zone == "public" ||
    context.device_trust == "unmanaged"
};
$cedar$, TRUE, 1, 'step_up_approval', 100),
-- Policy 3: Critical Risk → Deny Everything
('00000000-0000-0000-0000-000000000001', 'pol_deny_critical', 'forbid', '', $cedar$
@advice("deny_due_to_risk")
forbid(
    principal,
    action,
    resource
)
when {
    context.risk_band == "critical"
};
$cedar$, TRUE, 1, 'deny_due_to_risk', 100),
-- Policy 4: After Hours + On-Call/SRE → 30m JIT
('00000000-0000-0000-0000-000000000001', 'pol_after_hours_jit', 'permit', '', $cedar$
@advice("approve_30m_jit")
permit(
    principal,
    action == Action::"grant",
    resource
)
when {
    context.time_of_day == "after_hours" &&
    (context.role == "oncall" || context.role == "sre")
};
$cedar$, TRUE, 1, 'approve_30m_jit', 20)
ON CONFLICT (tenant_id, policy_id, version) DO NOTHING;

COMMIT;