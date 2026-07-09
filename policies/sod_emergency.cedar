// ─── SoD (Segregation of Duties) Policies ────────────────────
// Toxic combination detection

// Cannot both create a vendor AND approve invoices
// This assumes identity has paths through their role hierarchy
forbid(
    principal,
    action in [Action::"CreateVendor", Action::"ApproveInvoice"],
    resource
) when {
    // Evaluated by Temporal cron: if identity has THIS path and THAT path, it's toxic
    // The cron workflow marks identities violating this
    true
};

// Cannot have Prod DB access AND be a contractor
forbid(
    principal in Role::"Contractor",
    action == Action::"AdminAccess",
    resource
) when {
    resource.criticality in ["p0", "p1"]
};

// Emergency access requires immediate review
permit(
    principal,
    action == Action::"RevokeAccess",
    resource
) when {
    context.emergency == true
} unless {
    // Emergency access MUST be reviewed within 24 hours
    // This policy defers to workflow enforcement
    false
};

// Agent delegation: can only access less than what you have
permit(
    principal is Agent,
    action == Action::"DelegatedAccess",
    resource
) when {
    context.delegation_depth <= 3 &&
    context.original_principal != principal
};

// Agent tool calls: require active agent and bounded intent
permit(
    principal is Agent,
    action == Action::"ToolCall",
    resource
) when {
    principal.is_revoked == false &&
    context.tool_name != "dangerous"  // Replace with actual intent check
};
