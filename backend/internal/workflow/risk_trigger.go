package workflow

import (
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func RegisterRiskWorker(c client.Client, namespace string) error {
	w := worker.New(c, "risk-tasks", worker.Options{})
	w.RegisterWorkflow(RiskAlertWorkflow)
	log.Printf("[RISK-WORKFLOW] registered worker in namespace %s", namespace)
	return nil
}

func TriggerRiskAlert(c client.Client, namespace, identityID, eventType string, oldScore, newScore float64) error {
	if newScore < 60 {
		return nil
	}
	_, err := c.ExecuteWorkflow(nil, client.StartWorkflowOptions{
		TaskQueue: "risk-tasks",
	}, RiskAlertWorkflow, identityID, oldScore, newScore, eventType)
	return err
}
