// ─── ABAC Policies ───────────────────────────────────────────
// Attribute-based access control — fine-grained conditions

// Permit access if user belongs to the same department as the resource owner
permit(
    principal,
    action == Action::"ReadDocument",
    resource
) when {
    principal.department == resource.owner_department &&
    resource.data_classification != "critical"
};

// Permit cross-department access if user has sufficient clearance
permit(
    principal,
    action == Action::"ReadDocument",
    resource
) when {
    principal.clearance >= 5 &&
    resource.data_classification != "critical"
};

// Forbid access to PII/PHI data for contractors
forbid(
    principal in Role::"Contractor",
    action in [Action::"ReadAccess", Action::"ReadDocument"],
    resource
) when {
    resource.data_classification in ["pii", "phi", "pci"]
};

// Permit access only with MFA present for admin operations
permit(
    principal,
    action == Action::"AdminAccess",
    resource
) when {
    context.mfa_present == true &&
    context.session_age_ms < 3600000 &&  // session < 1 hour
    context.device_posture == "compliant"
};

// Permit cross-region access with MFA
permit(
    principal,
    action == Action::"ReadAccess",
    resource
) when {
    resource.region == principal.region ||
    context.mfa_present == true
};

// Contractor read access with MFA requirement for sensitive data
permit(
    principal in Role::"Contractor",
    action == Action::"ReadAccess",
    resource
) when {
    principal.is_active &&
    context.mfa_present == true &&
    resource.data_classification in ["medium", "low"]
};

// Forbid rubberband access (unused for 90 days)
// This is evaluated by the cron workflow, not inline,
// but shows how entropy-based policies work
forbid(
    principal,
    action,
    resource
) when {
    resource.criticality == "p0" &&
    principal.assurance_level < "aal2"
};
