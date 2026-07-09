package workflow

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/observeid/identity-platform/internal/domain"
)

// ─── Workflow Input Types ──────────────────────────────────

type OffboardInput struct {
	IdentityID        string   `json:"identity_id"`
	IdentityType      string   `json:"identity_type"`
	Reason            string   `json:"reason"`
	RequestedBy       string   `json:"requested_by"`
	SubjectsOfConcern []string `json:"subjects_of_concern"`
	TenantID          string   `json:"tenant_id"`
}

type OnboardInput struct {
	Email        string            `json:"email"`
	DisplayName  string            `json:"display_name"`
	IdentityType string            `json:"identity_type"`
	Department   string            `json:"department"`
	EmployeeID   string            `json:"employee_id"`
	ManagerID    string            `json:"manager_id"`
	Source       string            `json:"source"`
	RequestedBy  string            `json:"requested_by"`
	TenantID     string            `json:"tenant_id"`
	InitialRoles []string          `json:"initial_roles"`
	Attributes   map[string]string `json:"attributes"`
}

type GrantAccessInput struct {
	IdentityID       string `json:"identity_id"`
	ResourceID       string `json:"resource_id"`
	RoleID           string `json:"role_id"`
	RequestedBy      string `json:"requested_by"`
	DurationHours    int    `json:"duration_hours"`
	Reason           string `json:"reason"`
	TenantID         string `json:"tenant_id"`
	RequiresApproval bool   `json:"requires_approval"`
}

type RevokeAccessInput struct {
	IdentityID    string `json:"identity_id"`
	EntitlementID string `json:"entitlement_id"`
	Reason        string `json:"reason"`
	RevokedBy     string `json:"revoked_by"`
	IsEmergency   bool   `json:"is_emergency"`
	TenantID      string `json:"tenant_id"`
}

type JustInTimeInput struct {
	IdentityID    string `json:"identity_id"`
	ResourceID    string `json:"resource_id"`
	Reason        string `json:"reason"`
	RequestedBy   string `json:"requested_by"`
	DurationMins  int    `json:"duration_mins"`
	TenantID      string `json:"tenant_id"`
}

// ─── OffboardIdentityWorkflow ─────────────────────────────
// V1: Human and NHI offboarding with parallel fan-out, CAEP, audit
// V2: Cascade revoke delegation chains for AI agents

func OffboardIdentityWorkflow(ctx workflow.Context, input OffboardInput) error {
	v := workflow.GetVersion(ctx, "OffboardV2_NHI_Support", workflow.DefaultVersion, 1)
	logger := workflow.GetLogger(ctx)

	logger.Info("OffboardIdentityWorkflow started",
		"identity_id", input.IdentityID,
		"identity_type", input.IdentityType,
		"reason", input.Reason,
	)

	// Activity defaults
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 0: Create audit trail
	var auditID string
	if err := workflow.ExecuteActivity(ctx, "InitiateAuditTrail", map[string]any{
		"identity_id": input.IdentityID, "operation": "offboard",
		"reason": input.Reason, "requested_by": input.RequestedBy,
	}).Get(ctx, &auditID); err != nil {
		return fmt.Errorf("audit init failed: %w", err)
	}

	// Step 1: Acquire distributed lock
	var lockToken string
	if err := workflow.ExecuteActivity(ctx, "AcquireIdentityLock", map[string]any{
		"identity_id": input.IdentityID, "ttl_seconds": 120,
	}).Get(ctx, &lockToken); err != nil {
		return fmt.Errorf("lock acquisition failed: %w", err)
	}
	defer workflow.ExecuteActivity(ctx, "ReleaseIdentityLock", map[string]any{
		"identity_id": input.IdentityID, "token": lockToken,
	})

	// Step 2: Query Neo4j for all active entitlements
	var entitlements []domain.Entitlement
	if err := workflow.ExecuteActivity(ctx, "QueryIdentityEntitlements", map[string]any{
		"identity_id": input.IdentityID, "include_inherited": true,
	}).Get(ctx, &entitlements); err != nil {
		return fmt.Errorf("entitlement query failed: %w", err)
	}
	logger.Info("Found entitlements", "count", len(entitlements))

	// Step 3: Parallel fan-out — revoke each entitlement
	var futures []workflow.Future
	for _, e := range entitlements {
		f := workflow.ExecuteChildWorkflow(ctx,
			RevokeAccessChildWorkflow,
			RevokeAccessInput{
				IdentityID:    input.IdentityID,
				EntitlementID: e.ID,
				Reason:        input.Reason,
				RevokedBy:     input.RequestedBy,
				IsEmergency:   false,
				TenantID:      input.TenantID,
			},
		)
		futures = append(futures, f)
	}

	var failures []string
	for _, f := range futures {
		var result map[string]any
		if err := f.Get(ctx, &result); err != nil {
			failures = append(failures, err.Error())
		}
	}

	// Step 4: Cascade revoke delegated agents (NHI only — v2)
	if v >= 1 && input.IdentityType == "ai_agent" {
		var delegatedAgents []string
		if err := workflow.ExecuteActivity(ctx, "FindDelegatedAgents", map[string]any{
			"identity_id": input.IdentityID,
		}).Get(ctx, &delegatedAgents); err == nil {
			for _, agentID := range delegatedAgents {
				childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
					WorkflowID: fmt.Sprintf("cascade-revoke-%s", agentID),
					TaskQueue:  "critical_offboarding",
				})
				workflow.ExecuteChildWorkflow(childCtx, "RevokeAgentDelegationWorkflow", map[string]any{
					"agent_id": agentID, "revoked_by": input.IdentityID, "reason": "parent_deprovisioned",
				})
			}
		}
	}

	// Step 5: CAEP Broadcast
	if len(input.SubjectsOfConcern) > 0 {
		caepCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval: time.Second, BackoffCoefficient: 2.0, MaximumAttempts: 10,
			},
		})
		workflow.ExecuteActivity(caepCtx, "BroadcastCAEPEvent", map[string]any{
			"event_type": "session-revoked", "identity_id": input.IdentityID,
			"subjects": input.SubjectsOfConcern, "reason_admin": input.Reason,
		})
	}

	// Step 6: Finalize audit (write to QLDB)
	finalizeCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	})
	workflow.ExecuteActivity(finalizeCtx, "FinalizeAuditTrail", map[string]any{
		"audit_id": auditID, "status": "completed",
		"revoked_count": len(entitlements) - len(failures), "failure_count": len(failures),
	})

	logger.Info("OffboardIdentityWorkflow completed",
		"revoked", len(entitlements)-len(failures), "failed", len(failures))
	return nil
}

// ─── RevokeAccessChildWorkflow ────────────────────────────
// Single-system revocation with app-specific retry logic

func RevokeAccessChildWorkflow(ctx workflow.Context, input RevokeAccessInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Revoking access", "entitlement_id", input.EntitlementID)

	// App-specific activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:               time.Second,
			BackoffCoefficient:            2.0,
			MaximumInterval:               30 * time.Second,
			MaximumAttempts:               5,
			NonRetryableErrorTypes:        []string{"ForbiddenError", "NotFoundError"},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if err := workflow.ExecuteActivity(ctx, "RevokeTargetAccess", map[string]any{
		"identity_id": input.IdentityID, "entitlement_id": input.EntitlementID,
		"reason": input.Reason, "revoked_by": input.RevokedBy,
	}).Get(ctx, nil); err != nil {
		logger.Error("Revocation failed", "error", err)
		return err
	}

	return nil
}

// ─── OnboardIdentityWorkflow ───────────────────────────────

func OnboardIdentityWorkflow(ctx workflow.Context, input OnboardInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("OnboardIdentityWorkflow started", "email", input.Email)

	// Create identity in PostgreSQL + Neo4j
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var identityID string
	if err := workflow.ExecuteActivity(ctx, "CreateIdentity", map[string]any{
		"email": input.Email, "display_name": input.DisplayName,
		"identity_type": input.IdentityType, "department": input.Department,
		"employee_id": input.EmployeeID, "manager_id": input.ManagerID,
		"source": input.Source, "requested_by": input.RequestedBy,
		"tenant_id": input.TenantID, "initial_roles": input.InitialRoles,
	}).Get(ctx, &identityID); err != nil {
		return fmt.Errorf("identity creation failed: %w", err)
	}

	// Assign initial roles
	for _, roleName := range input.InitialRoles {
		if err := workflow.ExecuteActivity(ctx, "AssignRoleToIdentity", map[string]any{
			"identity_id": identityID, "role_name": roleName,
			"assigned_by": input.RequestedBy, "tenant_id": input.TenantID,
		}).Get(ctx, nil); err != nil {
			logger.Warn("Role assignment failed", "role", roleName, "error", err)
		}
	}

	logger.Info("Identity onboarded", "identity_id", identityID)
	return nil
}

// ─── GrantAccessWorkflow ──────────────────────────────────

func GrantAccessWorkflow(ctx workflow.Context, input GrantAccessInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("GrantAccessWorkflow started",
		"identity_id", input.IdentityID, "resource_id", input.ResourceID)

	// Approval gate
	if input.RequiresApproval {
		// Send approval request, wait for signal
		signalCh := workflow.GetSignalChannel(ctx, "ApprovalDecision")
		workflow.ExecuteActivity(ctx, "SendApprovalRequest", map[string]any{
			"identity_id": input.IdentityID, "requested_by": input.RequestedBy,
			"reason": input.Reason, "resource_id": input.ResourceID,
		})

		var approved bool
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &approved)
		})
		selector.Select(ctx)

		if !approved {
			logger.Info("Access grant denied by approver")
			return nil
		}
	}

	// Provision access
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if err := workflow.ExecuteActivity(ctx, "ProvisionAccess", map[string]any{
		"identity_id": input.IdentityID, "resource_id": input.ResourceID,
		"role_id": input.RoleID, "granted_by": input.RequestedBy,
		"duration_hours": input.DurationHours, "reason": input.Reason,
		"tenant_id": input.TenantID,
	}).Get(ctx, nil); err != nil {
		return fmt.Errorf("access provisioning failed: %w", err)
	}

	// If JIT, schedule automatic revocation
	if input.DurationHours > 0 {
		_ = workflow.NewTimer(ctx, time.Duration(input.DurationHours)*time.Hour)
		workflow.ExecuteChildWorkflow(ctx, RevokeAccessChildWorkflow, RevokeAccessInput{
			IdentityID: input.IdentityID, Reason: "jit_access_expired",
			RevokedBy: "system", IsEmergency: false, TenantID: input.TenantID,
		})
	}

	logger.Info("Access granted successfully")
	return nil
}

// ─── RevokeAccessWorkflow (Emergency) ─────────────────────

func RevokeAccessWorkflow(ctx workflow.Context, input RevokeAccessInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Warn("RevokeAccessWorkflow started",
		"identity_id", input.IdentityID, "emergency", input.IsEmergency)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if err := workflow.ExecuteActivity(ctx, "RevokeIdentityAccess", map[string]any{
		"identity_id": input.IdentityID, "entitlement_id": input.EntitlementID,
		"reason": input.Reason, "revoked_by": input.RevokedBy,
		"is_emergency": input.IsEmergency, "tenant_id": input.TenantID,
	}).Get(ctx, nil); err != nil {
		return fmt.Errorf("emergency revocation failed: %w", err)
	}

	// Broadcast CAEP for emergency revocations
	if input.IsEmergency {
		workflow.ExecuteActivity(ctx, "BroadcastCAEPEvent", map[string]any{
			"event_type": "session-revoked", "identity_id": input.IdentityID,
			"subjects": []string{input.IdentityID}, "reason_admin": input.Reason,
		})
	}

	return nil
}

// ─── JustInTimeAccessWorkflow ─────────────────────────────

func JustInTimeAccessWorkflow(ctx workflow.Context, input JustInTimeInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("JIT Access requested", "identity_id", input.IdentityID)

	// Validate with Cedar policy
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var allowed bool
	if err := workflow.ExecuteActivity(ctx, "CheckAccessPolicy", map[string]any{
		"identity_id": input.IdentityID, "resource_id": input.ResourceID,
		"action": "read", "tenant_id": input.TenantID,
	}).Get(ctx, &allowed); err != nil || !allowed {
		logger.Warn("JIT access denied by policy")
		return fmt.Errorf("access denied by policy")
	}

	// Grant time-bounded access
	grantCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})
	if err := workflow.ExecuteActivity(grantCtx, "ProvisionTemporaryAccess", map[string]any{
		"identity_id": input.IdentityID, "resource_id": input.ResourceID,
		"duration_minutes": input.DurationMins, "granted_by": input.RequestedBy,
		"reason": input.Reason, "tenant_id": input.TenantID,
	}).Get(ctx, nil); err != nil {
		return fmt.Errorf("temporary access provisioning failed: %w", err)
	}

	// Timer for automatic revocation
	timerCh := workflow.NewTimer(ctx, time.Duration(input.DurationMins)*time.Minute)
	_ = timerCh

	workflow.ExecuteActivity(ctx, "RevokeTemporaryAccess", map[string]any{
		"identity_id": input.IdentityID, "resource_id": input.ResourceID,
		"reason": "jit_expired", "revoked_by": "system",
	})

	logger.Info("JIT access expired and revoked")
	return nil
}

// ─── AgentAnomalyDetectionWorkflow (Cron) ─────────────────

func AgentAnomalyDetectionWorkflow(ctx workflow.Context) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Agent anomaly detection scan started")

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var anomalies []map[string]any
	if err := workflow.ExecuteActivity(ctx, "ScanAgentBehavior", nil).Get(ctx, &anomalies); err != nil {
		return fmt.Errorf("anomaly scan failed: %w", err)
	}

	for _, a := range anomalies {
		logger.Warn("Agent anomaly detected", "agent_id", a["agent_id"], "reason", a["reason"])
		if isCritical, _ := a["critical"].(bool); isCritical {
			workflow.ExecuteChildWorkflow(ctx, "AgentKillSwitchWorkflow", map[string]any{
				"agent_id": a["agent_id"], "reason": a["reason"].(string),
			})
		}
	}

	logger.Info("Anomaly detection complete", "anomalies_found", len(anomalies))
	return nil
}

// ─── SoD Detection Workflow (Cron) ────────────────────────

func DetectSoDViolationsWorkflow(ctx workflow.Context) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("SoD violation scan started")

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var violations []map[string]any
	if err := workflow.ExecuteActivity(ctx, "ScanSoDViolations", nil).Get(ctx, &violations); err != nil {
		return fmt.Errorf("SoD scan failed: %w", err)
	}

	logger.Info("SoD scan complete", "violations_found", len(violations))
	return nil
}
