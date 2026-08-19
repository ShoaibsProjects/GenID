package workflow

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ConnectorSyncScheduleInput drives a scheduled connector full-sync.
// The ScheduleCron from the connector config drives the Temporal
// Schedule; this workflow executes one tick.
type ConnectorSyncScheduleInput struct {
	ConnectorID string `json:"connector_id"`
	TenantID    string `json:"tenant_id"`
}

// ConnectorSyncScheduleWorkflow performs a single scheduled sync tick.
// It delegates to the connector manager's full-sync path so the sync
// remains the authoritative operation (not duplicated in workflow code).
func ConnectorSyncScheduleWorkflow(ctx workflow.Context, input ConnectorSyncScheduleInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Scheduled connector sync started",
		"connector_id", input.ConnectorID,
		"tenant_id", input.TenantID,
	)

	// The sync itself is executed as an activity so it lives in the
	// activity worker (not the workflow replay) and can call out to
	// external systems (Graph, DB) safely.
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    30 * time.Second,
			BackoffCoefficient: 1.5,
			MaximumInterval:    5 * time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var syncResult struct {
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	if err := workflow.ExecuteActivity(ctx, "RunConnectorFullSync",
		map[string]any{
			"connector_id": input.ConnectorID,
			"tenant_id":    input.TenantID,
		},
	).Get(ctx, &syncResult); err != nil {
		return fmt.Errorf("scheduled sync failed: %w", err)
	}

	logger.Info("Scheduled connector sync completed", "status", syncResult.Status)
	return nil
}
