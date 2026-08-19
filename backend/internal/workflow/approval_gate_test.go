package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/observeid/genid/internal/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

// ─── ApprovalGateWorkflow ───────────────────────────────────

// Helper: standard mock store activities with a configurable chain.
func setupApprovalGateEnv(t *testing.T, approvals []*Approval) *testsuite.TestWorkflowEnvironment {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(approvalChainActivity, activity.RegisterOptions{Name: "CreateApprovalChain"})
	env.RegisterActivityWithOptions(decideApprovalActivity, activity.RegisterOptions{Name: "DecideApproval"})
	env.RegisterActivityWithOptions(appendAuditActivity, activity.RegisterOptions{Name: "AppendAudit"})
	env.RegisterActivityWithOptions(notifyApprovalActivity, activity.RegisterOptions{Name: "NotifyApproval"})

	env.OnActivity("CreateApprovalChain", mock.Anything, mock.Anything, mock.Anything).
		Return(approvals, nil)

	for _, a := range approvals {
		env.OnActivity("DecideApproval", mock.Anything, a.ID, mock.Anything, mock.Anything, mock.Anything).
			Return(a, nil)
	}
	env.OnActivity("AppendAudit", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	env.OnActivity("NotifyApproval", mock.Anything, mock.Anything).
		Return(nil)

	return env
}

func TestApprovalGate_SequentialBothApprove(t *testing.T) {
	approvals := []*Approval{
		{ID: "ap-1", RequestID: "req-1", Level: 1, ApproverRole: "resource_owner", Status: "pending", DueAt: ptr(time.Now().Add(24 * time.Hour))},
		{ID: "ap-2", RequestID: "req-1", Level: 2, ApproverRole: "security_admin", Status: "pending", DueAt: ptr(time.Now().Add(8 * time.Hour))},
	}
	env := setupApprovalGateEnv(t, approvals)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ApprovalSignalName, ApprovalDecision{ApprovalID: "ap-1", ApproverID: "user-owner", Approved: true})
	}, 0)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ApprovalSignalName, ApprovalDecision{ApprovalID: "ap-2", ApproverID: "user-sec", Approved: true})
	}, 0)

	var result *ApprovalGateResult
	env.ExecuteWorkflow(ApprovalGateWorkflow, ApprovalGateInput{
		RequestID: "req-1",
		Steps: []ApprovalStep{
			{Level: 1, ApproverRole: "resource_owner", Mode: "sequential", DueInHours: 24},
			{Level: 2, ApproverRole: "security_admin", Mode: "sequential", DueInHours: 8},
		},
	})
	require.NoError(t, env.GetWorkflowResult(&result))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	t.Logf("result: %+v", result)
	assert.True(t, result.Approved)
}

func TestApprovalGate_SequentialDeniedAtLevel2(t *testing.T) {
	approvals := []*Approval{
		{ID: "ap-1", RequestID: "req-1", Level: 1, ApproverRole: "resource_owner", Status: "pending", DueAt: ptr(time.Now().Add(24 * time.Hour))},
		{ID: "ap-2", RequestID: "req-1", Level: 2, ApproverRole: "security_admin", Status: "pending", DueAt: ptr(time.Now().Add(8 * time.Hour))},
	}
	env := setupApprovalGateEnv(t, approvals)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ApprovalSignalName, ApprovalDecision{ApprovalID: "ap-1", ApproverID: "user-owner", Approved: true})
	}, 0)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ApprovalSignalName, ApprovalDecision{ApprovalID: "ap-2", ApproverID: "user-sec", Approved: false, Comment: "not justified"})
	}, 0)

	var result *ApprovalGateResult
	env.ExecuteWorkflow(ApprovalGateWorkflow, ApprovalGateInput{
		RequestID: "req-1",
		Steps: []ApprovalStep{
			{Level: 1, ApproverRole: "resource_owner", Mode: "sequential", DueInHours: 24},
			{Level: 2, ApproverRole: "security_admin", Mode: "sequential", DueInHours: 8},
		},
	})
	require.NoError(t, env.GetWorkflowResult(&result))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	assert.False(t, result.Approved)
	assert.Equal(t, 2, result.DeniedLevel)
	assert.Equal(t, "user-sec", result.DeniedBy)
}

func TestApprovalGate_ParallelAllApprove(t *testing.T) {
	approvals := []*Approval{
		{ID: "ap-1", RequestID: "req-1", Level: 1, ApproverRole: "resource_owner", Status: "pending", DueAt: ptr(time.Now().Add(24 * time.Hour))},
		{ID: "ap-2", RequestID: "req-1", Level: 1, ApproverRole: "compliance_officer", Status: "pending", DueAt: ptr(time.Now().Add(24 * time.Hour))},
	}
	env := setupApprovalGateEnv(t, approvals)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ApprovalSignalName, ApprovalDecision{ApprovalID: "ap-2", ApproverID: "user-comp", Approved: true})
	}, 0)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(ApprovalSignalName, ApprovalDecision{ApprovalID: "ap-1", ApproverID: "user-owner", Approved: true})
	}, 0)

	var result *ApprovalGateResult
	env.ExecuteWorkflow(ApprovalGateWorkflow, ApprovalGateInput{
		RequestID: "req-1",
		Steps: []ApprovalStep{
			{Level: 1, ApproverRole: "resource_owner", Mode: "parallel", DueInHours: 24},
			{Level: 1, ApproverRole: "compliance_officer", Mode: "parallel", DueInHours: 24},
		},
	})
	require.NoError(t, env.GetWorkflowResult(&result))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	t.Logf("result: %+v", result)
	assert.True(t, result.Approved)
}

func TestApprovalGate_EmptyChainAutoApproves(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(approvalChainActivity, activity.RegisterOptions{Name: "CreateApprovalChain"})
	env.OnActivity("CreateApprovalChain", mock.Anything, mock.Anything, mock.Anything).Return([]*Approval{}, nil)

	var result *ApprovalGateResult
	env.ExecuteWorkflow(ApprovalGateWorkflow, ApprovalGateInput{
		RequestID: "req-1",
		Steps:     []ApprovalStep{},
	})
	require.NoError(t, env.GetWorkflowResult(&result))

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	t.Logf("result: %+v", result)
	assert.True(t, result.Approved)
}

// ─── Test activity stand-ins (replaced by mocks at runtime) ──

func approvalChainActivity(ctx context.Context, requestID string, steps []ApprovalStep) ([]*Approval, error) {
	return nil, nil
}

func decideApprovalActivity(ctx context.Context, approvalID, approverID, status, comment string) (*Approval, error) {
	return nil, nil
}

func appendAuditActivity(ctx context.Context, requestID, step, actor string, details any) error {
	return nil
}

func notifyApprovalActivity(ctx context.Context, ev notify.Event) error {
	return nil
}
func TestApprovalGate_DeadlineTimesOut(t *testing.T) {
	approvals := []*Approval{
		{ID: "ap-1", RequestID: "req-1", Level: 1, ApproverRole: "resource_owner", Status: "pending", DueAt: ptr(time.Now().Add(1 * time.Hour))},
	}
	env := setupApprovalGateEnv(t, approvals)

	// No signals — deadline passes → auto-deny.
	var result *ApprovalGateResult
	env.ExecuteWorkflow(ApprovalGateWorkflow, ApprovalGateInput{
		RequestID: "req-1",
		Steps: []ApprovalStep{
			{Level: 1, ApproverRole: "resource_owner", Mode: "sequential", DueInHours: 24},
		},
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.False(t, result.Approved)
	assert.Equal(t, 1, result.DeniedLevel)
	assert.Equal(t, "approval timed out", result.Reason)
}
