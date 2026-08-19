package workflow

import (
	"fmt"
	"time"

	"github.com/observeid/genid/internal/notify"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ─── ApprovalGateWorkflow ────────────────────────────────────
// Child workflow executed by GrantAccessWorkflow (and friends)
// when the routed chain requires human approval.
//
// It persists the chain via the store activity, then waits for
// ApprovalDecision signals. Sequential chains open one level at a
// time; parallel chains open all levels at once and require every
// one to approve. A pending step that misses its due_at deadline is
// auto-denied ("approval timed out").

type ApprovalDecision struct {
	ApprovalID string `json:"approval_id"`
	ApproverID string `json:"approver_id"`
	Approved   bool   `json:"approved"`
	Comment    string `json:"comment"`
}

type ApprovalGateInput struct {
	RequestID string
	Steps     []ApprovalStep
}

type ApprovalGateResult struct {
	Approved    bool
	DeniedLevel int
	DeniedBy    string
	Reason      string
}

// ApprovalSignalName is the signal name used to submit decisions.
const ApprovalSignalName = "ApprovalDecision"

func ApprovalGateWorkflow(ctx workflow.Context, input ApprovalGateInput) (*ApprovalGateResult, error) {
	logger := workflow.GetLogger(ctx)

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})

	// Persist the routed chain (idempotent in the store); returns
	// rows with generated approval ids to wait on.
	var approvals []*Approval
	if err := workflow.ExecuteActivity(actCtx, "CreateApprovalChain", input.RequestID, input.Steps).Get(ctx, &approvals); err != nil {
		return nil, fmt.Errorf("persist approval chain: %w", err)
	}

	if len(approvals) == 0 {
		return &ApprovalGateResult{Approved: true}, nil
	}

	signalCh := workflow.GetSignalChannel(ctx, ApprovalSignalName)

	// Group persisted rows by level; rows sharing a level are parallel.
	byLevel := map[int][]*Approval{}
	levels := []int{}
	for _, a := range approvals {
		if _, ok := byLevel[a.Level]; !ok {
			levels = append(levels, a.Level)
		}
		byLevel[a.Level] = append(byLevel[a.Level], a)
	}

	for _, lvl := range levels {
		steps := byLevel[lvl]

		logger.Info("approval level open", "level", lvl, "steps", len(steps))

		// Notify approvers that their decision is required.
		for _, a := range steps {
			_ = workflow.ExecuteActivity(actCtx, "NotifyApproval", notify.Event{
				Event:         "approval.required",
				RequestID:     a.RequestID,
				RequestType:   a.RequestType,
				ApprovalID:    a.ID,
				Level:         a.Level,
				ApproverRole:  a.ApproverRole,
				ApproverEmail: a.ApproverEmail,
				DueAt:         formatRFC3339(a.DueAt),
			}).Get(ctx, nil)
		}

		// Deadline = soonest due_at among the level's rows.
		deadline := 24 * time.Hour
		for _, a := range steps {
			if a.DueAt != nil {
				d := a.DueAt.Sub(time.Now())
				if d > 0 && d < deadline {
					deadline = d
				}
			}
		}

		pending := map[string]bool{}
		for _, a := range steps {
			pending[a.ID] = true
		}

		// One deadline timer + one selector for the whole level.
		timer := workflow.NewTimer(ctx, deadline)
		selector := workflow.NewSelector(ctx)
		selector.AddFuture(timer, func(f workflow.Future) {
			_ = f.Get(ctx, nil)
		})
		var decision ApprovalDecision
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &decision)
		})
		timerFired := false

		// Wait for every step in this level (timer or decisions).
		for len(pending) > 0 && !timerFired {
			decision = ApprovalDecision{}
			selector.Select(ctx)

			if timer.IsReady() {
				timerFired = true
				break
			}
			if decision.ApprovalID == "" {
				continue
			}
			if !pending[decision.ApprovalID] {
				continue
			}

			status := "denied"
			if decision.Approved {
				status = "approved"
			}
			decisionErr := workflow.ExecuteActivity(actCtx, "DecideApproval",
				decision.ApprovalID, decision.ApproverID, status, decision.Comment).Get(ctx, nil)
			if decisionErr != nil {
				// The API may have persisted the decision before the
				// signal arrived. Re-read the row to reconcile.
				var row *Approval
				if err := workflow.ExecuteActivity(actCtx, "GetApproval", decision.ApprovalID).Get(ctx, &row); err != nil {
					logger.Warn("decision not recorded", "approval_id", decision.ApprovalID, "error", err)
					continue
				}
				if row.Status == "pending" {
					logger.Warn("decision rejected while still pending", "approval_id", decision.ApprovalID, "error", decisionErr)
					continue
				}
				if row.Status == "denied" {
					decision.Approved = false
				} else {
					decision.Approved = true
				}
			}
			delete(pending, decision.ApprovalID)

			if !decision.Approved {
				_ = workflow.ExecuteActivity(actCtx, "AppendAudit", input.RequestID, "approval.denied",
					"user:"+decision.ApproverID, map[string]any{
						"level":   lvl,
						"comment": decision.Comment,
					}).Get(ctx, nil)
				_ = workflow.ExecuteActivity(actCtx, "NotifyApproval", notify.Event{
					Event:      "approval.decided",
					RequestID:  input.RequestID,
					ApprovalID: decision.ApprovalID,
					Level:      lvl,
					ApproverID: decision.ApproverID,
					Status:     "denied",
					Comment:    decision.Comment,
					DecidedAt:  time.Now().UTC().Format(time.RFC3339),
				}).Get(ctx, nil)
				return &ApprovalGateResult{
					Approved:    false,
					DeniedLevel: lvl,
					DeniedBy:    decision.ApproverID,
					Reason:      "denied by approver",
				}, nil
			}

			_ = workflow.ExecuteActivity(actCtx, "NotifyApproval", notify.Event{
				Event:      "approval.decided",
				RequestID:  input.RequestID,
				ApprovalID: decision.ApprovalID,
				Level:      lvl,
				ApproverID: decision.ApproverID,
				Status:     "approved",
				Comment:    decision.Comment,
				DecidedAt:  time.Now().UTC().Format(time.RFC3339),
			}).Get(ctx, nil)
		}

		if timerFired {
			_ = workflow.ExecuteActivity(actCtx, "AppendAudit", input.RequestID, "approval.timed_out",
				"system", map[string]any{"level": lvl, "reason": "approval deadline exceeded"}).Get(ctx, nil)
			_ = workflow.ExecuteActivity(actCtx, "NotifyApproval", notify.Event{
				Event:      "approval.timed_out",
				RequestID:  input.RequestID,
				Level:      lvl,
				ApproverID: "system",
				Status:     "denied",
				Comment:    "approval deadline exceeded",
			}).Get(ctx, nil)
			return &ApprovalGateResult{
				Approved:    false,
				DeniedLevel: lvl,
				Reason:      "approval timed out",
			}, nil
		}

		_ = workflow.ExecuteActivity(actCtx, "AppendAudit", input.RequestID, "approval.completed",
			"system", map[string]any{"level": lvl}).Get(ctx, nil)
	}

	return &ApprovalGateResult{Approved: true}, nil
}

func formatRFC3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
