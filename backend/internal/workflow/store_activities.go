package workflow

import (
	"context"

	"github.com/observeid/genid/internal/notify"
)

// ─── Store-backed Activities ─────────────────────────────────
// Bridge between Temporal workflows and the workflow_requests DB
// store. Registered on the worker alongside the main activity
// service; methods are referenced by name from workflows
// (e.g. "CreateApprovalChain", "DecideApproval", "AppendAudit").

type StoreActivities struct {
	store    *Store
	notifier *notify.Notifier
}

func NewStoreActivities(store *Store) *StoreActivities {
	return &StoreActivities{store: store}
}

// NewStoreActivitiesWithNotifier attaches the webhook notifier
// (optional; approvals still work without it).
func NewStoreActivitiesWithNotifier(store *Store, n *notify.Notifier) *StoreActivities {
	return &StoreActivities{store: store, notifier: n}
}

// CreateApprovalChain persists the routed chain for a request and
// returns the resulting rows (with generated IDs) so the workflow
// can wait on the exact approval ids. Idempotent: existing rows are
// returned as-is.
func (s *StoreActivities) CreateApprovalChain(ctx context.Context, requestID string, steps []ApprovalStep) ([]*Approval, error) {
	if _, err := s.store.CreateApprovalChain(ctx, requestID, steps); err != nil {
		return nil, err
	}
	return s.store.ListApprovals(ctx, requestID)
}

// DecideApproval records one approver decision.
func (s *StoreActivities) DecideApproval(ctx context.Context, approvalID, approverID, status, comment string) (*Approval, error) {
	return s.store.DecideApproval(ctx, approvalID, approverID, status, comment)
}

// GetApproval re-reads one approval row (used to reconcile decisions
// that the API already persisted before the signal reached the gate).
func (s *StoreActivities) GetApproval(ctx context.Context, approvalID string) (*Approval, error) {
	return s.store.GetApproval(ctx, approvalID)
}

// AppendAudit writes one workflow audit entry.
func (s *StoreActivities) AppendAudit(ctx context.Context, requestID, step, actor string, details any) error {
	return s.store.AppendAudit(ctx, requestID, step, actor, details)
}

// UpdateRequestStatus transitions the request row.
func (s *StoreActivities) UpdateRequestStatus(ctx context.Context, id, status string) error {
	return s.store.UpdateRequestStatus(ctx, id, status)
}

// UpdateRequestFailureReason attaches a reason to a denied/failed row.
func (s *StoreActivities) UpdateRequestFailureReason(ctx context.Context, id, reason string) error {
	return s.store.UpdateRequestFailureReason(ctx, id, reason)
}

// NotifyApproval delivers an approval event to configured webhooks
// (Slack/Teams). Best-effort: never fails the workflow.
func (s *StoreActivities) NotifyApproval(ctx context.Context, ev notify.Event) error {
	if s.notifier == nil {
		return nil
	}
	return s.notifier.Notify(ctx, ev)
}
