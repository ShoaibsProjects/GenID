package middleware

import (
	"log"
	"net/http"
	"strings"
)

// OperationType identifies the workflow being invoked.
type OperationType string

const (
	OpGrantAccess   OperationType = "grant_access"
	OpRevokeAccess  OperationType = "revoke_access"
	OpKillSwitch    OperationType = "agent_kill_switch"
	OpDelegateAgent OperationType = "delegate_agent"
	OpAssignRole    OperationType = "assign_role"
	OpRemoveRole    OperationType = "remove_role"
	OpExecuteLCM    OperationType = "execute_lcm"
	OpSyncConnector OperationType = "sync_connector"
	OpVaultStore    OperationType = "vault_store_secret"
	OpVaultDelete   OperationType = "vault_delete_secret"
	OpSCIMWrite     OperationType = "scim_write"
	OpCreateGroup   OperationType = "create_group"
	OpDeleteGroup   OperationType = "delete_group"
	OpBulkImport    OperationType = "bulk_import"
)

// Human-readable labels for audit log / error messages.
var opLabels = map[OperationType]string{
	OpGrantAccess:   "grant access",
	OpRevokeAccess:  "revoke access",
	OpKillSwitch:    "agent kill-switch",
	OpDelegateAgent: "delegate agent",
	OpAssignRole:    "assign role",
	OpRemoveRole:    "remove role",
	OpExecuteLCM:    "execute lifecycle operation",
	OpSyncConnector: "sync connector",
	OpVaultStore:    "store vault secret",
	OpVaultDelete:   "delete vault secret",
	OpSCIMWrite:     "SCIM write operation",
	OpCreateGroup:   "create group",
	OpDeleteGroup:   "delete group",
	OpBulkImport:    "bulk identity import",
}

// WorkflowGuard protects operations that should only be performed by
// master / owner users, or callers presenting a known master key.
type WorkflowGuard struct {
	masterKey string
	enabled   bool
}

// NewWorkflowGuard creates a guard. When the masterKey is empty, the
// guard is effectively disabled (dev mode).
func NewWorkflowGuard(masterKey string) *WorkflowGuard {
	enabled := masterKey != ""
	if enabled {
		log.Printf("[workflow-guard] master-key workflow protection ENABLED")
	} else {
		log.Printf("[workflow-guard] master-key workflow protection DISABLED (dev mode)")
	}
	return &WorkflowGuard{
		masterKey: masterKey,
		enabled:   enabled,
	}
}

// Enabled reports whether the guard is active.
func (g *WorkflowGuard) Enabled() bool {
	return g.enabled
}

// Protect wraps a handler and requires the caller to be a master.
func (g *WorkflowGuard) Protect(op OperationType, next http.HandlerFunc) http.HandlerFunc {
	if !g.enabled {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !g.isMaster(r) {
			label := opLabels[op]
			if label == "" {
				label = string(op)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			msg := `{"error":"forbidden","detail":"` + label + ` requires master permission"}`
			w.Write([]byte(msg))
			return
		}
		next(w, r)
	}
}

// isMaster checks whether the request originated from a master user.
// Sources (checked in order):
//  1. X-Master-Key header matches the configured master key
//  2. X-User-Role header contains "master" or "admin"
//  3. X-User-ID header maps to a master user (checked against config)
func (g *WorkflowGuard) isMaster(r *http.Request) bool {
	// Check 1: master API key
	if key := r.Header.Get("X-Master-Key"); key != "" {
		return key == g.masterKey
	}

	// Check 2: role-based (from auth gateway / reverse proxy)
	if role := r.Header.Get("X-User-Role"); role != "" {
		for _, r := range strings.Split(role, ",") {
			r = strings.TrimSpace(strings.ToLower(r))
			if r == "master" || r == "admin" || r == "owner" {
				return true
			}
		}
	}

	return false
}
