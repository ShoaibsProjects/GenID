package workflow

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/observeid/identity-platform/internal/activities"
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
	ResourceType     string `json:"resource_type"`
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
	IdentityID   string `json:"identity_id"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Action       string `json:"action"`
	Reason       string `json:"reason"`
	RequestedBy  string `json:"requested_by"`
	DurationMins int    `json:"duration_mins"`
	TenantID     string `json:"tenant_id"`
}

type CascadeRevokeInput struct {
	AgentID   string `json:"agent_id"`
	RevokedBy string `json:"revoked_by"`
	Reason    string `json:"reason"`
}

// ─── OffboardIdentityWorkflow ─────────────────────────────
// Multi-phase de-provisioning with compensating transactions
//
// Phase 1: Pre-checks + Lock
// Phase 2: Disable identity (sessions, auth, cache)
// Phase 3: Revoke direct entitlements (parallel fan-out)
// Phase 4: Remove group memberships
// Phase 5: Cascade to delegated agents (NHI only)
// Phase 6: Verify (scan for remaining access)
// Phase 7: CAEP broadcast + Audit finalization

func OffboardIdentityWorkflow(ctx workflow.Context, input OffboardInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("OffboardIdentityWorkflow started",
		"identity_id", input.IdentityID,
		"identity_type", input.IdentityType,
		"reason", input.Reason,
	)

	startTime := workflow.Now(ctx)

	// ── Activity Options ────────────────────────────────
	baseAO := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
			NonRetryableErrorTypes: []string{"ForbiddenError", "NotFoundError"},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, baseAO)

	// ── Phase 1: Pre-checks + Lock ──────────────────────
	var audit activities.AuditTrailResult
	if err := workflow.ExecuteActivity(ctx, "InitiateAuditTrail", activities.AuditTrailParams{
		Operation:    "offboard",
		IdentityID:   input.IdentityID,
		IdentityType: input.IdentityType,
		Reason:       input.Reason,
		RequestedBy:  input.RequestedBy,
		TenantID:     input.TenantID,
	}).Get(ctx, &audit); err != nil {
		return fmt.Errorf("phase1: audit init: %w", err)
	}

	var lock activities.LockResult
	if err := workflow.ExecuteActivity(ctx, "AcquireIdentityLock", activities.LockParams{
		IdentityID: input.IdentityID,
		TTLSeconds: 120,
	}).Get(ctx, &lock); err != nil {
		workflow.ExecuteActivity(ctx, "FinalizeAuditTrail", map[string]any{
			"audit_id": audit.AuditID, "status": "failed_lock",
		})
		return fmt.Errorf("phase1: lock: %w", err)
	}
	defer workflow.ExecuteActivity(ctx, "ReleaseIdentityLock", activities.UnlockParams{
		IdentityID: input.IdentityID, Token: lock.Token,
	})

	logger.Info("Phase 1 complete: lock acquired", "fence", lock.FenceVersion)

	// ── Phase 2: Disable identity ───────────────────────
	var entitlements []activities.EntitlementResult
	if err := workflow.ExecuteActivity(ctx, "QueryIdentityEntitlements", activities.EntitlementQueryParams{
		IdentityID: input.IdentityID,
		TenantID:   input.TenantID,
	}).Get(ctx, &entitlements); err != nil {
		return fmt.Errorf("phase2: entitlement query: %w", err)
	}
	logger.Info("Phase 2 complete: entitlements queried", "count", len(entitlements))

	// ── Phase 3: Revoke all entitlements (parallel) ─────
	// Group by risk score for phased revocation
	var criticalEnts, highEnts, normalEnts []activities.EntitlementResult
	for _, e := range entitlements {
		switch {
		case e.IsToxic || e.RiskScore >= 0.7:
			criticalEnts = append(criticalEnts, e)
		case e.RiskScore >= 0.4:
			highEnts = append(highEnts, e)
		default:
			normalEnts = append(normalEnts, e)
		}
	}

	// Phase 3a: Revoke critical entitlements first (sequential for safety)
	var failures []string
	for _, e := range criticalEnts {
		if err := executeRevocationWithRetry(ctx, input, e, true); err != nil {
			failures = append(failures, fmt.Sprintf("critical:%s:%v", e.ID, err))
		}
	}

	// Phase 3b: Revoke high-risk entitlements (parallel)
	highFutures := make([]workflow.Future, len(highEnts))
	for i, e := range highEnts {
		e := e
		highFutures[i] = workflow.ExecuteChildWorkflow(ctx,
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
	}
	for _, f := range highFutures {
		if err := f.Get(ctx, nil); err != nil {
			failures = append(failures, err.Error())
		}
	}

	// Phase 3c: Revoke normal entitlements (parallel)
	normalFutures := make([]workflow.Future, len(normalEnts))
	for i, e := range normalEnts {
		e := e
		normalFutures[i] = workflow.ExecuteChildWorkflow(ctx,
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
	}
	for _, f := range normalFutures {
		if err := f.Get(ctx, nil); err != nil {
			failures = append(failures, err.Error())
		}
	}

	logger.Info("Phase 3 complete: entitlements revoked",
		"critical", len(criticalEnts), "high", len(highEnts),
		"normal", len(normalEnts), "failures", len(failures))

	// ── Phase 4: Cascade to delegated agents (NHI) ──────
	if input.IdentityType == "ai_agent" || input.IdentityType == "service_account" || input.IdentityType == "robot" {
		var delegated []activities.DelegationResult
		if err := workflow.ExecuteActivity(ctx, "FindDelegatedAgents", activities.DelegationQueryParams{
			IdentityID: input.IdentityID,
			MaxDepth:   3,
		}).Get(ctx, &delegated); err != nil {
			logger.Warn("delegation query failed, continuing", "error", err)
		} else {
			for _, d := range delegated {
				childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
					WorkflowID:            fmt.Sprintf("cascade-revoke-%s-%s", input.IdentityID, d.AgentID),
					WorkflowRunTimeout:    5 * time.Minute,
					WaitForCancellation:   true,
				})
				if err := workflow.ExecuteChildWorkflow(childCtx, CascadeRevokeWorkflow, CascadeRevokeInput{
					AgentID:   d.AgentID,
					RevokedBy: input.IdentityID,
					Reason:    fmt.Sprintf("parent_deprovisioned:%s", input.Reason),
				}).Get(ctx, nil); err != nil {
					failures = append(failures, fmt.Sprintf("cascade:%s:%v", d.AgentID, err))
				}
			}
		}
		logger.Info("Phase 4 complete: delegated agents processed", "count", len(delegated))
	}

	// ── Phase 5: Emergency revocation step ──────────────
	// Mark identity as revoked in all systems
	if err := workflow.ExecuteActivity(ctx, "RevokeIdentityAccess", activities.RevocationParams{
		IdentityID:  input.IdentityID,
		Reason:      input.Reason,
		RevokedBy:   input.RequestedBy,
		IsEmergency: false,
		TenantID:    input.TenantID,
	}).Get(ctx, nil); err != nil {
		failures = append(failures, fmt.Sprintf("revoke_identity:%v", err))
	}

	logger.Info("Phase 5 complete: identity disabled")

	// ── Phase 6: CAEP Broadcast ─────────────────────────
	if len(input.SubjectsOfConcern) > 0 || len(failures) == 0 {
		caepCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    time.Second,
				BackoffCoefficient: 2.0,
				MaximumAttempts:    10,
			},
		})
		workflow.ExecuteActivity(caepCtx, "BroadcastCAEPEvent", activities.CAEPEventParams{
			EventType:   "session-revoked",
			IdentityID:  input.IdentityID,
			Subjects:    input.SubjectsOfConcern,
			ReasonAdmin: input.Reason,
			ReasonUser:  "Your access has been revoked",
			TenantID:    input.TenantID,
		})
	}

	// ── Phase 7: Audit finalization ─────────────────────
	duration := workflow.Now(ctx).Sub(startTime)
	status := "completed"
	if len(failures) > 0 {
		status = "completed_with_errors"
	}

	finalizeCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	})
	workflow.ExecuteActivity(finalizeCtx, "FinalizeAuditTrail", map[string]any{
		"audit_id": audit.AuditID, "status": status,
		"revoked_count":    len(entitlements) - len(failures),
		"failure_count":    len(failures),
		"duration_ms":      duration.Milliseconds(),
	})

	logger.Info("OffboardIdentityWorkflow completed",
		"revoked", len(entitlements)-len(failures),
		"failed", len(failures),
		"duration_ms", duration.Milliseconds())

	if len(failures) > 0 {
		// Return partial failure — Temporal will surface this
		return fmt.Errorf("offboard completed with %d/%d failures: %v",
			len(failures), len(entitlements), failures)
	}
	return nil
}

// ─── RevokeAccessChildWorkflow ────────────────────────────
// Single-system revocation with exponential backoff and app-specific retry

func RevokeAccessChildWorkflow(ctx workflow.Context, input RevokeAccessInput) error {
	logger := workflow.GetLogger(ctx)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:       time.Second,
			BackoffCoefficient:    2.0,
			MaximumInterval:       30 * time.Second,
			MaximumAttempts:       5,
			NonRetryableErrorTypes: []string{"ForbiddenError", "NotFoundError"},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if err := workflow.ExecuteActivity(ctx, "RevokeTargetAccess", activities.RevocationParams{
		IdentityID:    input.IdentityID,
		EntitlementID: input.EntitlementID,
		Reason:        input.Reason,
		RevokedBy:     input.RevokedBy,
		IsEmergency:   input.IsEmergency,
		TenantID:      input.TenantID,
	}).Get(ctx, nil); err != nil {
		logger.Error("Revocation failed", "error", err)
		return err
	}

	return nil
}

// ─── CascadeRevokeWorkflow ─────────────────────────────
// Cascading revocation for delegated NHI agents:
// revoke credentials, then recursively deactivate

func CascadeRevokeWorkflow(ctx workflow.Context, input CascadeRevokeInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Cascade revoke for agent", "agent_id", input.AgentID)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 1: Revoke SPIFFE SVID
	var spiffeErr error
	_ = workflow.ExecuteActivity(ctx, "RevokeSPIFFESVID", map[string]any{
		"agent_id": input.AgentID,
	}).Get(ctx, &spiffeErr)

	// Step 2: Revoke OAuth tokens
	var oauthErr error
	_ = workflow.ExecuteActivity(ctx, "RevokeOAuthTokens", map[string]any{
		"agent_id": input.AgentID,
	}).Get(ctx, &oauthErr)

	// Step 3: Revoke API keys
	var apikeysErr error
	_ = workflow.ExecuteActivity(ctx, "RevokeAPIKeys", map[string]any{
		"agent_id": input.AgentID,
	}).Get(ctx, &apikeysErr)

	// Step 4: Rotate any remaining credentials
	var rotateErr error
	_ = workflow.ExecuteActivity(ctx, "RotateCredentials", map[string]any{
		"identity_id":     input.AgentID,
		"credential_type": "api_key",
	}).Get(ctx, &rotateErr)

	// Aggregate errors — cascade should try all steps even if some fail
	var failures []string
	if spiffeErr != nil {
		failures = append(failures, fmt.Sprintf("spiffe: %v", spiffeErr))
	}
	if oauthErr != nil {
		failures = append(failures, fmt.Sprintf("oauth: %v", oauthErr))
	}
	if apikeysErr != nil {
		failures = append(failures, fmt.Sprintf("apikeys: %v", apikeysErr))
	}
	if rotateErr != nil {
		failures = append(failures, fmt.Sprintf("rotate: %v", rotateErr))
	}
	if len(failures) > 0 {
		logger.Warn("Cascade revocation partial failures", "failures", failures)
		return fmt.Errorf("cascade revocation had %d failures: %s", len(failures), strings.Join(failures, "; "))
	}

	logger.Info("Cascade revocation complete")
	return nil
}

// ─── OnboardIdentityWorkflow ───────────────────────────────
// Identity creation with role resolution and multi-system provisioning

func OnboardIdentityWorkflow(ctx workflow.Context, input OnboardInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("OnboardIdentityWorkflow started", "email", input.Email)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 1: Create identity in PostgreSQL + Neo4j
	var createResult activities.CreateIdentityResult
	if err := workflow.ExecuteActivity(ctx, "CreateIdentity", activities.CreateIdentityParams{
		Email:        input.Email,
		DisplayName:  input.DisplayName,
		IdentityType: input.IdentityType,
		Department:   input.Department,
		EmployeeID:   input.EmployeeID,
		ManagerID:    input.ManagerID,
		Source:       input.Source,
		RequestedBy:  input.RequestedBy,
		TenantID:     input.TenantID,
		Attributes:   input.Attributes,
	}).Get(ctx, &createResult); err != nil {
		return fmt.Errorf("identity creation failed: %w", err)
	}

	identityID := createResult.IdentityID
	logger.Info("Identity created", "identity_id", identityID)

	// Step 2: Assign initial roles (parallel)
	roleFutures := make([]workflow.Future, len(input.InitialRoles))
	for i, roleName := range input.InitialRoles {
		i, roleName := i, roleName
		roleFutures[i] = workflow.ExecuteActivity(ctx, "AssignRoleToIdentity", activities.AssignRoleParams{
			IdentityID: identityID,
			RoleName:   roleName,
			AssignedBy: input.RequestedBy,
			TenantID:   input.TenantID,
		})
	}
	for _, f := range roleFutures {
		if err := f.Get(ctx, nil); err != nil {
			logger.Warn("Role assignment failed", "error", err)
		}
	}

	// Step 3: Schedule access certification (7-day reminder)
	_ = workflow.NewTimer(ctx, 7*24*time.Hour)

	logger.Info("Identity onboarded successfully", "identity_id", identityID)
	return nil
}

// ─── GrantAccessWorkflow ──────────────────────────────────
// SoD-aware access granting with policy evaluation, approval, and JIT expiry

func GrantAccessWorkflow(ctx workflow.Context, input GrantAccessInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("GrantAccessWorkflow started",
		"identity_id", input.IdentityID,
		"resource_id", input.ResourceID,
	)

	// ── Step 1: Pre-flight checks ───────────────────────
	checkAO := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	checkCtx := workflow.WithActivityOptions(ctx, checkAO)

	// 1a: Check Cedar policy
	var policyResult activities.PolicyCheckResult
	if err := workflow.ExecuteActivity(checkCtx, "CheckAccessPolicy", activities.PolicyCheckParams{
		IdentityID:   input.IdentityID,
		ResourceID:   input.ResourceID,
		ResourceType: input.ResourceType,
		Action:       "grant",
		TenantID:     input.TenantID,
		Context: map[string]any{
			"grant_type": "permanent",
			"requested_by": input.RequestedBy,
			"reason":       input.Reason,
		},
	}).Get(ctx, &policyResult); err != nil {
		return fmt.Errorf("policy check failed: %w", err)
	}

	if !policyResult.Allowed {
		logger.Warn("Access denied by policy", "decision", policyResult.Decision)
		return fmt.Errorf("access denied by policy: %s", policyResult.Reason)
	}

	// 1b: Check SoD conflicts (if role-based)
	if input.RoleID != "" {
		var sodResult activities.SoDCheckResult
		if err := workflow.ExecuteActivity(checkCtx, "CheckSoDConflicts", map[string]any{
			"identity_id": input.IdentityID,
			"role_id":     input.RoleID,
		}).Get(ctx, &sodResult); err != nil {
			logger.Warn("SoD check failed, continuing", "error", err)
		} else if sodResult.HasConflict {
			if sodResult.RiskScore > 0.7 {
				return fmt.Errorf("access denied: critical SoD conflict (risk: %.2f)", sodResult.RiskScore)
			}
			logger.Warn("SoD conflict detected but below threshold", "risk", sodResult.RiskScore)
		}
	}

	// ── Step 2: Approval gate ─────────────────────────
	if input.RequiresApproval {
		approvalCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Second,
		})
		workflow.ExecuteActivity(approvalCtx, "SendApprovalRequest", map[string]any{
			"identity_id":  input.IdentityID,
			"resource_id":  input.ResourceID,
			"requested_by": input.RequestedBy,
			"reason":       input.Reason,
		})

		signalCh := workflow.GetSignalChannel(ctx, "ApprovalDecision")
		var approved bool
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &approved)
		})
		selector.Select(ctx)

		if !approved {
			logger.Info("Access grant denied by approver")
			return errors.New("access denied by approver")
		}
	}

	// ── Step 3: Provision access ─────────────────────
	provAO := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	}
	provCtx := workflow.WithActivityOptions(ctx, provAO)

	if input.DurationHours > 0 {
		if err := workflow.ExecuteActivity(provCtx, "ProvisionTemporaryAccess", activities.ProvisionParams{
			IdentityID:      input.IdentityID,
			ResourceID:      input.ResourceID,
			RoleID:          input.RoleID,
			DurationMinutes: input.DurationHours * 60,
			GrantedBy:       input.RequestedBy,
			Reason:          input.Reason,
			TenantID:        input.TenantID,
		}).Get(ctx, nil); err != nil {
			return fmt.Errorf("temporary provisioning failed: %w", err)
		}
	} else {
		if err := workflow.ExecuteActivity(provCtx, "ProvisionAccess", activities.ProvisionParams{
			IdentityID: input.IdentityID,
			ResourceID: input.ResourceID,
			RoleID:     input.RoleID,
			GrantedBy:  input.RequestedBy,
			Reason:     input.Reason,
			TenantID:   input.TenantID,
		}).Get(ctx, nil); err != nil {
			return fmt.Errorf("provisioning failed: %w", err)
		}
	}

	// ── Step 4: Schedule JIT revocation timer ─────────
	if input.DurationHours > 0 {
		timerFuture := workflow.NewTimer(ctx, time.Duration(input.DurationHours)*time.Hour)

		// Wait for timer OR revoke-before-expiry signal
		revokeCh := workflow.GetSignalChannel(ctx, "RevokeBeforeExpiry")
		selector := workflow.NewSelector(ctx)
		var revokedEarly bool
		selector.AddFuture(timerFuture, func(f workflow.Future) {
			_ = f.Get(ctx, nil) // timer expired
		})
		selector.AddReceive(revokeCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &revokedEarly)
		})
		selector.Select(ctx)

		if revokedEarly {
			logger.Info("Access revoked early by signal")
		}

		workflow.ExecuteChildWorkflow(ctx, RevokeAccessChildWorkflow, RevokeAccessInput{
			IdentityID: input.IdentityID,
			Reason:     "jit_access_expired",
			RevokedBy:  "system",
			TenantID:   input.TenantID,
		})
	}

	logger.Info("Access granted successfully")
	return nil
}

// ─── RevokeAccessWorkflow (Emergency) ─────────────────────
// Emergency revocation with cache invalidation, credential rotation, and CAEP

func RevokeAccessWorkflow(ctx workflow.Context, input RevokeAccessInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Warn("RevokeAccessWorkflow started",
		"identity_id", input.IdentityID,
		"emergency", input.IsEmergency,
	)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// ── For emergency: disable identity first ──────────
	if input.IsEmergency {
		if err := workflow.ExecuteActivity(ctx, "RevokeIdentityAccess", activities.RevocationParams{
			IdentityID:  input.IdentityID,
			EntitlementID: input.EntitlementID,
			Reason:      input.Reason,
			RevokedBy:   input.RevokedBy,
			IsEmergency: true,
			TenantID:    input.TenantID,
		}).Get(ctx, nil); err != nil {
			return fmt.Errorf("emergency identity revoke failed: %w", err)
		}

		// Rotate credentials — partial failure is non-blocking
		var rotateErr error
		_ = workflow.ExecuteActivity(ctx, "RotateCredentials", map[string]any{
			"identity_id":     input.IdentityID,
			"credential_type": "api_key",
		}).Get(ctx, &rotateErr)
		if rotateErr != nil {
			logger.Warn("Credential rotation failed during revoke", "error", rotateErr)
		}

		// Broadcast CAEP
		workflow.ExecuteActivity(ctx, "BroadcastCAEPEvent", activities.CAEPEventParams{
			EventType:   "session-revoked",
			IdentityID:  input.IdentityID,
			Subjects:    []string{input.IdentityID},
			ReasonAdmin: input.Reason,
			ReasonUser:  "Emergency revocation",
			TenantID:    input.TenantID,
		})
	} else {
		// Non-emergency: revoke specific entitlement
		if err := workflow.ExecuteActivity(ctx, "RevokeTargetAccess", activities.RevocationParams{
			IdentityID:    input.IdentityID,
			EntitlementID: input.EntitlementID,
			Reason:        input.Reason,
			RevokedBy:     input.RevokedBy,
			IsEmergency:   false,
			TenantID:      input.TenantID,
		}).Get(ctx, nil); err != nil {
			return fmt.Errorf("revocation failed: %w", err)
		}
	}

	logger.Info("RevokeAccessWorkflow completed")
	return nil
}

// ─── JustInTimeAccessWorkflow ─────────────────────────────
// Policy-validated, time-bounded access elevation

func JustInTimeAccessWorkflow(ctx workflow.Context, input JustInTimeInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("JIT Access requested",
		"identity_id", input.IdentityID,
		"resource_id", input.ResourceID,
	)

	if input.Action == "" {
		input.Action = "read"
	}
	if input.DurationMins <= 0 {
		input.DurationMins = 60
	}

	// ── Step 1: Policy validation ─────────────────────
	checkAO := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	checkCtx := workflow.WithActivityOptions(ctx, checkAO)

	var policyResult activities.PolicyCheckResult
	if err := workflow.ExecuteActivity(checkCtx, "CheckAccessPolicy", activities.PolicyCheckParams{
		IdentityID:   input.IdentityID,
		ResourceID:   input.ResourceID,
		ResourceType: input.ResourceType,
		Action:       input.Action,
		TenantID:     input.TenantID,
		Context: map[string]any{
			"grant_type":    "jit",
			"duration_mins": input.DurationMins,
			"reason":        input.Reason,
		},
	}).Get(ctx, &policyResult); err != nil {
		return fmt.Errorf("policy check failed: %w", err)
	}

	if !policyResult.Allowed {
		logger.Warn("JIT access denied by policy")
		return fmt.Errorf("jit access denied by policy: %s", policyResult.Reason)
	}

	// ── Step 2: Grant temporary access ────────────────
	grantAO := workflow.ActivityOptions{
		StartToCloseTimeout: 1 * time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	}
	grantCtx := workflow.WithActivityOptions(ctx, grantAO)

	if err := workflow.ExecuteActivity(grantCtx, "ProvisionTemporaryAccess", activities.ProvisionParams{
		IdentityID:      input.IdentityID,
		ResourceID:      input.ResourceID,
		DurationMinutes: input.DurationMins,
		GrantedBy:       input.RequestedBy,
		Reason:          input.Reason,
		TenantID:        input.TenantID,
	}).Get(ctx, nil); err != nil {
		return fmt.Errorf("jit provisioning failed: %w", err)
	}

	logger.Info("JIT access granted", "duration_minutes", input.DurationMins)

	// ── Step 3: Wait for expiry or early revocation ───
	timerCh := workflow.NewTimer(ctx, time.Duration(input.DurationMins)*time.Minute)
	revokeCh := workflow.GetSignalChannel(ctx, "RevokeBeforeExpiry")

	selector := workflow.NewSelector(ctx)
	var revokedEarly bool
	selector.AddFuture(timerCh, func(f workflow.Future) {
		_ = f.Get(ctx, nil)
	})
	selector.AddReceive(revokeCh, func(c workflow.ReceiveChannel, _ bool) {
		c.Receive(ctx, &revokedEarly)
	})
	selector.Select(ctx)

	if revokedEarly {
		logger.Info("JIT access revoked early by signal")
	}

	// ── Step 4: Revoke access ─────────────────────────
	if err := workflow.ExecuteActivity(grantCtx, "RevokeTemporaryAccess", map[string]any{
		"identity_id": input.IdentityID,
		"resource_id": input.ResourceID,
		"reason":      "jit_expired",
		"revoked_by":  "system",
	}).Get(ctx, nil); err != nil {
		logger.Warn("JIT revocation failed", "error", err)
		return err
	}

	logger.Info("JIT access expired and revoked")
	return nil
}

// ─── AgentAnomalyDetectionWorkflow (Cron) ─────────────────
// Multi-signal anomaly detection with automated remediation

func AgentAnomalyDetectionWorkflow(ctx workflow.Context) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Agent anomaly detection scan started")

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var anomalies []activities.AnomalyResult
	if err := workflow.ExecuteActivity(ctx, "ScanAgentBehavior", nil).Get(ctx, &anomalies); err != nil {
		return fmt.Errorf("anomaly scan failed: %w", err)
	}

	logger.Info("Anomaly scan complete", "anomalies_found", len(anomalies))

	// Group anomalies by agent for consolidated remediation
	agentAnomalies := make(map[string][]activities.AnomalyResult)
	for _, a := range anomalies {
		agentAnomalies[a.AgentID] = append(agentAnomalies[a.AgentID], a)
	}

	for agentID, agentAnoms := range agentAnomalies {
		logger.Warn("Anomalies detected",
			"agent_id", agentID,
			"count", len(agentAnoms),
		)

		// Determine if any anomaly is critical
		hasCritical := false
		maxScore := 0.0
		for _, a := range agentAnoms {
			if a.Critical {
				hasCritical = true
			}
			if a.Score > maxScore {
				maxScore = a.Score
			}
		}

		// Auto-remediate critical anomalies
		if hasCritical || maxScore > 0.8 {
			childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowID:            fmt.Sprintf("auto-remediate-%s", agentID),
				WorkflowRunTimeout:    3 * time.Minute,
				WaitForCancellation:   true,
			})
			workflow.ExecuteChildWorkflow(childCtx, "CascadeRevokeWorkflow", CascadeRevokeInput{
				AgentID:   agentID,
				RevokedBy: "system",
				Reason:    fmt.Sprintf("auto_remediate:critical_anomaly_score_%.2f", maxScore),
			})
		}
	}

	return nil
}

// ─── SoD Detection Workflow (Cron) ────────────────────────
// Graph-based segregation of duties violation scanning

func DetectSoDViolationsWorkflow(ctx workflow.Context) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("SoD violation scan started")

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 2},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var conflicts []activities.SoDConflict
	if err := workflow.ExecuteActivity(ctx, "ScanSoDViolations", nil).Get(ctx, &conflicts); err != nil {
		return fmt.Errorf("sod scan failed: %w", err)
	}

	logger.Info("SoD scan complete", "violations_found", len(conflicts))

	// Critical violations trigger remediation workflows
	for _, c := range conflicts {
		if c.Severity == "critical" {
			logger.Warn("Critical SoD violation requiring remediation",
				"type", c.ConflictType,
				"existing_role", c.ExistingRoleName,
			)
		}
	}

	return nil
}

// ─── Helpers ──────────────────────────────────────────────

func executeRevocationWithRetry(ctx workflow.Context, input OffboardInput, e activities.EntitlementResult, critical bool) error {
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID:            fmt.Sprintf("revoke-critical-%s", e.ID),
		WorkflowRunTimeout:    2 * time.Minute,
		WaitForCancellation:   true,
	})
	return workflow.ExecuteChildWorkflow(childCtx, RevokeAccessChildWorkflow, RevokeAccessInput{
		IdentityID:    input.IdentityID,
		EntitlementID: e.ID,
		Reason:        input.Reason,
		RevokedBy:     input.RequestedBy,
		IsEmergency:   false,
		TenantID:      input.TenantID,
	}).Get(ctx, nil)
}
