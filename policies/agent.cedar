// ─── Agent / NHI Policies ────────────────────────────────────
// AI Agent and Non-Human Identity authorization

// Permit MCP tool calls for registered agents
permit(
    principal is Agent,
    action == Action::"ToolCall",
    resource
) when {
    principal.is_revoked == false &&
    context.mcp_session_id != "" &&
    context.intent != "" &&
    principal.deployment_env == "production"
};

// Forbid agents from accessing critical systems directly
forbid(
    principal is Agent,
    action in [Action::"AdminAccess", Action::"RevokeAccess"],
    resource
) when {
    resource.criticality in ["p0"] &&
    context.delegation_depth == 0
};

// Permit delegated agent access with scope narrowing
permit(
    principal is Agent,
    action in [Action::"ReadAccess", Action::"DelegatedAccess"],
    resource
) when {
    principal.is_revoked == false &&
    context.delegation_depth >= 1 &&
    context.delegation_depth <= 3
};

// Service accounts: explicit permit with constraints
permit(
    principal is NonHumanIdentity,
    action in [Action::"ReadAccess"],
    resource
) when {
    principal.is_governed == true &&
    resource.data_classification != "critical"
};

// Unmanaged (shadow) NHIs: deny by default
forbid(
    principal is NonHumanIdentity,
    action,
    resource
) when {
    principal.is_governed == false
};

// RPA bots: allow only when human-supervised
permit(
    principal is NonHumanIdentity,
    action == Action::"WriteDocument",
    resource
) when {
    principal.type == "rpa_bot" &&
    principal.is_governed == true &&
    resource.data_classification != "critical"
};
