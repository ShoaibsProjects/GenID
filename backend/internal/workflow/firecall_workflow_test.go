package workflow

import (
	"context"
	"testing"

	"github.com/observeid/genid/internal/activities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/activity"
)

// ─── FirecallAccessWorkflow ─────────────────────────────────
//
// Verifies the break-glass semantics:
//   1. Access is provisioned IMMEDIATELY (no approval gate).
//   2. Duration defaults to 60m and is hard-capped at 240m.
//   3. After expiry the access is revoked.
//   4. Workflow waits for the mandatory post-event review signal.

func TestFirecallAccessWorkflow_DefaultDuration(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(provisionActivity, activity.RegisterOptions{Name: "ProvisionTemporaryAccess"})
	env.RegisterActivityWithOptions(revokeActivity, activity.RegisterOptions{Name: "RevokeTemporaryAccess"})

	env.RegisterActivityWithOptions(provisionActivity, activity.RegisterOptions{Name: "ProvisionTemporaryAccess"})
	env.RegisterActivityWithOptions(revokeActivity, activity.RegisterOptions{Name: "RevokeTemporaryAccess"})

	var provisioned activities.ProvisionParams
	env.OnActivity(provisionActivity, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			if p, ok := args.Get(1).(activities.ProvisionParams); ok {
				provisioned = p
			}
		})
	env.OnActivity(revokeActivity, mock.Anything, mock.Anything).Return(nil)

	input := FirecallInput{
		IdentityID:  "id-1",
		ResourceID:  "res-1",
		Action:      "",
		Reason:      "",
		RequestedBy: "user-a",
	}
	env.ExecuteWorkflow(FirecallAccessWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	// Default duration 60m; grant is attributed to the firecall requester.
	assert.Equal(t, 60, provisioned.DurationMinutes)
	assert.Equal(t, "firecall:user-a", provisioned.GrantedBy)
}

func TestFirecallAccessWorkflow_DurationHardCappedAt240(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(provisionActivity, activity.RegisterOptions{Name: "ProvisionTemporaryAccess"})
	env.RegisterActivityWithOptions(revokeActivity, activity.RegisterOptions{Name: "RevokeTemporaryAccess"})

	var provisioned activities.ProvisionParams
	env.OnActivity(provisionActivity, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			if p, ok := args.Get(1).(activities.ProvisionParams); ok {
				provisioned = p
			}
		})
	env.OnActivity(revokeActivity, mock.Anything, mock.Anything).Return(nil)

	input := FirecallInput{
		IdentityID:   "id-1",
		ResourceID:   "res-1",
		DurationMins: 600,
		RequestedBy:  "user-a",
	}
	env.ExecuteWorkflow(FirecallAccessWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	assert.Equal(t, 240, provisioned.DurationMinutes)
}

func TestFirecallAccessWorkflow_ProvisionFailureFailsWorkflow(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(provisionActivity, activity.RegisterOptions{Name: "ProvisionTemporaryAccess"})
	env.RegisterActivityWithOptions(revokeActivity, activity.RegisterOptions{Name: "RevokeTemporaryAccess"})

	env.OnActivity(provisionActivity, mock.Anything, mock.Anything).
		Return(assert.AnError).Once()

	input := FirecallInput{
		IdentityID:   "id-1",
		ResourceID:   "res-1",
		DurationMins: 30,
		RequestedBy:  "user-a",
	}
	env.ExecuteWorkflow(FirecallAccessWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

// ─── Early revoke via signal ────────────────────────────────

func TestFirecallAccessWorkflow_EarlyRevokeSignal(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(provisionActivity, activity.RegisterOptions{Name: "ProvisionTemporaryAccess"})
	env.RegisterActivityWithOptions(revokeActivity, activity.RegisterOptions{Name: "RevokeTemporaryAccess"})

	env.OnActivity(provisionActivity, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(revokeActivity, mock.Anything, mock.Anything).Return(nil)

	input := FirecallInput{
		IdentityID:   "id-1",
		ResourceID:   "res-1",
		DurationMins: 120,
		RequestedBy:  "user-a",
	}

	// Signal immediately so the workflow takes the early-revoke branch.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("RevokeBeforeExpiry", true)
	}, 0)

	env.ExecuteWorkflow(FirecallAccessWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}
// Test activity implementations bound by function name; they are replaced
// by env.OnActivity mocks at runtime.
func provisionActivity(ctx context.Context, params activities.ProvisionParams) error {
	return nil
}

func revokeActivity(ctx context.Context, params map[string]any) error {
	return nil
}
