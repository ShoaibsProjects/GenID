// default_policy.cedar — flagship conditional-access policies.
//
// Source of truth for the seeded rows in migration 008. The engine loads
// policies from cedar_policies (cedar_text); keep this file in sync with
// the migration.
//
// The Cedar language has no `advice` keyword, so each policy carries the
// routing hint as a standard @advice("...") annotation, which cedar-go
// exposes via Policy.Annotations(). The engine reads it to decide
// Allow-with-duration / StepUp / Deny.

// Policy 1: IT Admin + Corporate + Business Hours + Managed + Low Risk → Auto-Approve 2h JIT
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

// Policy 2: Public Network OR Unmanaged Device → Step-Up Approval
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

// Policy 3: Critical Risk → Deny Everything
@advice("deny_due_to_risk")
forbid(
    principal,
    action,
    resource
)
when {
    context.risk_band == "critical"
};

// Policy 4: After Hours + On-Call/SRE → 30m JIT
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