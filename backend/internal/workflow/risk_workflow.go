package workflow

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

func RiskAlertWorkflow(ctx workflow.Context, identityID string, oldScore, newScore float64, eventType string) error {
	logger := workflow.GetLogger(ctx)
	band := scoreToBand(newScore)
	oldBand := scoreToBand(oldScore)

	logger.Info("Risk alert workflow started",
		"identityID", identityID,
		"oldScore", oldScore,
		"newScore", newScore,
		"band", band,
		"eventType", eventType,
	)

	if band == "minimal" || band == "low" {
		logger.Info("Risk below threshold, no action", "band", band)
		return nil
	}

	if oldBand == band {
		logger.Info("Risk band unchanged, no escalation", "band", band)
		return nil
	}

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	switch band {
	case "critical":
		logger.Warn("CRITICAL RISK: auto-revoke triggered", "identityID", identityID, "score", newScore)
		if err := executeRevoke(ctx, identityID); err != nil {
			return fmt.Errorf("auto-revoke failed: %w", err)
		}
	case "high":
		logger.Warn("HIGH RISK: step-up MFA required", "identityID", identityID, "score", newScore)
		if err := executeStepUpMFA(ctx, identityID); err != nil {
			return fmt.Errorf("step-up MFA failed: %w", err)
		}
	case "elevated":
		logger.Warn("ELEVATED RISK: alert sent", "identityID", identityID, "score", newScore)
		if err := executeAlert(ctx, identityID, newScore, eventType); err != nil {
			return fmt.Errorf("alert failed: %w", err)
		}
	}
	return nil
}

func scoreToBand(score float64) string {
	switch {
	case score >= 800:
		return "critical"
	case score >= 600:
		return "high"
	case score >= 300:
		return "elevated"
	case score >= 100:
		return "low"
	default:
		return "minimal"
	}
}

func executeRevoke(ctx workflow.Context, identityID string) error {
	return workflow.ExecuteActivity(ctx, func(id string) error {
		fmt.Printf("[RISK-WORKFLOW] AUTO-REVOKE identity=%s\n", id)
		return nil
	}, identityID).Get(ctx, nil)
}

func executeStepUpMFA(ctx workflow.Context, identityID string) error {
	return workflow.ExecuteActivity(ctx, func(id string) error {
		fmt.Printf("[RISK-WORKFLOW] STEP-UP MFA identity=%s\n", id)
		return nil
	}, identityID).Get(ctx, nil)
}

func executeAlert(ctx workflow.Context, identityID string, score float64, eventType string) error {
	return workflow.ExecuteActivity(ctx, func(id string, s float64, e string) error {
		fmt.Printf("[RISK-WORKFLOW] ALERT identity=%s score=%.0f event=%s\n", id, s, e)
		return nil
	}, identityID, score, eventType).Get(ctx, nil)
}
