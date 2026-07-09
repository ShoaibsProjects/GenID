// ─── RBAC Policies ───────────────────────────────────────────
// Role-based access control

// Administrators have full access
permit(
    principal in Role::"Administrator",
    action,
    resource
);

// Engineers have standard read/write access
permit(
    principal in Role::"Engineer",
    action in [Action::"ReadAccess", Action::"ReadDocument", Action::"WriteDocument"],
    resource
);

// Auditors have read-only access to everything
forbid(
    principal in Role::"Auditor",
    action in [Action::"WriteDocument", Action::"AdminAccess", Action::"RevokeAccess"],
    resource
);

permit(
    principal in Role::"Auditor",
    action in [Action::"ReadAccess", Action::"ReadDocument"],
    resource
);

// Contractors have limited access (deny-overrides model)
forbid(
    principal in Role::"Contractor",
    action in [Action::"AdminAccess", Action::"RevokeAccess"],
    resource
);

permit(
    principal in Role::"Contractor",
    action in [Action::"ReadAccess", Action::"ReadDocument"],
    resource
) when {
    principal.is_active
};
