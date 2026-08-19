package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ─── Workflow DB Store ────────────────────────────────────────
// CRUD for workflow_requests / workflow_approvals / workflow_audit.
// All Temporal workflows in this package write through this store so the
// UI/API can inspect status, approvals, and full audit trail.

type Request struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	Type               string          `json:"type"`
	Status             string          `json:"status"`
	RequesterID        string          `json:"requester_id"`
	TargetID           string          `json:"target_id"`
	Payload            json.RawMessage `json:"payload"`
	IdempotencyKey     string          `json:"idempotency_key,omitempty"`
	TemporalWorkflowID string          `json:"temporal_workflow_id,omitempty"`
	TemporalRunID      string          `json:"temporal_run_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	ExpiresAt          *time.Time      `json:"expires_at,omitempty"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	FailureReason      string          `json:"failure_reason,omitempty"`
}

type Approval struct {
	ID            string     `json:"id"`
	RequestID     string     `json:"request_id"`
	Level         int        `json:"level"`
	ApproverID    string     `json:"approver_id"`
	ApproverEmail string     `json:"approver_email"`
	ApproverRole  string     `json:"approver_role"`
	Status        string     `json:"status"`
	Comment       string     `json:"comment"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	DueAt         *time.Time `json:"due_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	RequestType   string     `json:"request_type,omitempty"`
	RequestStatus string     `json:"request_status,omitempty"`
}

type AuditEntry struct {
	ID        int64           `json:"id"`
	RequestID string          `json:"request_id"`
	Step      string          `json:"step"`
	Actor     string          `json:"actor"`
	Details   json.RawMessage `json:"details"`
	TS        time.Time       `json:"ts"`
}

// DBPool is the minimal pgx surface the store needs, so tests can inject
// pgxmock instead of a real connection pool.
type DBPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	pool DBPool
}

func NewStore(pool DBPool) *Store {
	return &Store{pool: pool}
}

// ─── Requests ──────────────────────────────────────────────────

// CreateRequest inserts a request row. On idempotency-key conflict it
// returns the EXISTING row's ID (created=false) so callers can short-circuit
// duplicate submissions instead of starting orphan workflows.
func (s *Store) CreateRequest(ctx context.Context, r *Request) (created bool, err error) {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Status == "" {
		r.Status = "pending"
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	var idemKey *string
	if r.IdempotencyKey != "" {
		k := r.IdempotencyKey
		idemKey = &k
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO workflow_requests (
			id, tenant_id, type, status, requester_id, target_id, payload,
			idempotency_key, temporal_workflow_id, temporal_run_id,
			created_at, updated_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (tenant_id, type, idempotency_key)
		WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
		DO NOTHING
		RETURNING id
	`, r.ID, r.TenantID, r.Type, r.Status,
		nullString(r.RequesterID), nullString(r.TargetID), r.Payload,
		idemKey, nullString(r.TemporalWorkflowID), nullString(r.TemporalRunID),
		r.CreatedAt, r.UpdatedAt, r.ExpiresAt).Scan(&r.ID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("create request: %w", err)
	}

	// Conflict: fetch the existing row for this tenant+type+key.
	if r.IdempotencyKey == "" {
		return false, fmt.Errorf("create request: insert returned no row without idempotency key")
	}
	err = s.pool.QueryRow(ctx, `
		SELECT id FROM workflow_requests
		WHERE tenant_id = $1 AND type = $2 AND idempotency_key = $3
		LIMIT 1
	`, r.TenantID, r.Type, r.IdempotencyKey).Scan(&r.ID)
	if err != nil {
		return false, fmt.Errorf("create request: conflict lookup: %w", err)
	}
	return false, nil
}

func (s *Store) GetRequest(ctx context.Context, id string) (*Request, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, type, status, COALESCE(requester_id::text, ''),
		       COALESCE(target_id::text, ''), payload, COALESCE(idempotency_key, ''),
		       COALESCE(temporal_workflow_id, ''), COALESCE(temporal_run_id, ''),
		       created_at, updated_at, expires_at, completed_at, COALESCE(failure_reason, '')
		FROM workflow_requests WHERE id = $1
	`, id)
	var r Request
	var payload []byte
	err := row.Scan(&r.ID, &r.TenantID, &r.Type, &r.Status, &r.RequesterID, &r.TargetID,
		&payload, &r.IdempotencyKey, &r.TemporalWorkflowID, &r.TemporalRunID,
		&r.CreatedAt, &r.UpdatedAt, &r.ExpiresAt, &r.CompletedAt, &r.FailureReason)
	if err != nil {
		return nil, err
	}
	r.Payload = payload
	return &r, nil
}

func (s *Store) ListRequests(ctx context.Context, status, reqType string, limit int) ([]*Request, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, tenant_id, type, status, COALESCE(requester_id::text, ''),
	             COALESCE(target_id::text, ''), payload, COALESCE(idempotency_key, ''),
	             COALESCE(temporal_workflow_id, ''), COALESCE(temporal_run_id, ''),
	             created_at, updated_at, expires_at, completed_at, COALESCE(failure_reason, '')
	      FROM workflow_requests WHERE 1=1`
	args := []any{}
	idx := 1
	if status != "" {
		q += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if reqType != "" {
		q += fmt.Sprintf(" AND type = $%d", idx)
		args = append(args, reqType)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Request
	for rows.Next() {
		var r Request
		var payload []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Type, &r.Status, &r.RequesterID, &r.TargetID,
			&payload, &r.IdempotencyKey, &r.TemporalWorkflowID, &r.TemporalRunID,
			&r.CreatedAt, &r.UpdatedAt, &r.ExpiresAt, &r.CompletedAt, &r.FailureReason); err != nil {
			return nil, err
		}
		r.Payload = payload
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateRequestStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE workflow_requests
		SET status = $1, updated_at = NOW(),
		    completed_at = CASE WHEN status IN ('executed','denied','failed','cancelled')
		                     THEN NOW() ELSE completed_at END
		WHERE id = $2
	`, status, id)
	return err
}

// UpdateRequestFailureReason attaches a human-readable reason to a
// denied/failed request row (e.g. "denied at level 2 by security_admin").
func (s *Store) UpdateRequestFailureReason(ctx context.Context, id, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE workflow_requests SET failure_reason = $1, updated_at = NOW()
		WHERE id = $2
	`, reason, id)
	return err
}

func (s *Store) SetTemporalIDs(ctx context.Context, id, workflowID, runID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE workflow_requests SET temporal_workflow_id=$1, temporal_run_id=$2, updated_at=NOW()
		WHERE id=$3
	`, workflowID, runID, id)
	return err
}

func (s *Store) FailRequest(ctx context.Context, id, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE workflow_requests SET status='failed', failure_reason=$1, completed_at=NOW(), updated_at=NOW()
		WHERE id=$2
	`, reason, id)
	return err
}

// ─── Approvals ─────────────────────────────────────────────────

func (s *Store) CreateApproval(ctx context.Context, a *Approval) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	a.CreatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workflow_approvals (id, request_id, level, approver_id, approver_email,
			approver_role, status, due_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, a.ID, a.RequestID, a.Level,
		nullString(a.ApproverID), a.ApproverEmail, a.ApproverRole,
		a.Status, a.DueAt, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("create approval: %w", err)
	}
	return nil
}

func (s *Store) ListApprovals(ctx context.Context, requestID string) ([]*Approval, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, request_id, level, COALESCE(approver_id::text, ''),
		       COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		       status, COALESCE(comment, ''), decided_at, due_at, created_at
		FROM workflow_approvals WHERE request_id = $1 ORDER BY level
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.RequestID, &a.Level, &a.ApproverID,
			&a.ApproverEmail, &a.ApproverRole, &a.Status, &a.Comment,
			&a.DecidedAt, &a.DueAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// CreateApprovalChain persists the routed chain for a request.
// Idempotent: if approvals already exist for the request, it is a
// no-op (returns the existing count).
func (s *Store) CreateApprovalChain(ctx context.Context, requestID string, steps []ApprovalStep) (int, error) {
	existing, err := s.ListApprovals(ctx, requestID)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return len(existing), nil
	}
	now := time.Now().UTC()
	for _, st := range steps {
		a := &Approval{
			ID:           uuid.NewString(),
			RequestID:    requestID,
			Level:        st.Level,
			ApproverRole: st.ApproverRole,
			Status:       "pending",
			DueAt:        ptr(DueAt(now, st)),
		}
		if err := s.CreateApproval(ctx, a); err != nil {
			return 0, err
		}
	}
	return len(steps), nil
}

// DecideApproval records an approver's decision on one approval row
// and returns the updated row. Decisions are immutable: a row already
// decided returns an error.
func (s *Store) DecideApproval(ctx context.Context, approvalID, approverID, decision, comment string) (*Approval, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE workflow_approvals
		SET status = $1, comment = $2, decided_at = NOW(),
		    approver_id = COALESCE(approver_id, $3::uuid)
		WHERE id = $4 AND status = 'pending'
		RETURNING id, request_id, level, COALESCE(approver_id::text, ''),
		          COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		          status, COALESCE(comment, ''), decided_at, due_at, created_at
	`, decision, comment, nullString(approverID), approvalID)
	var a Approval
	err := row.Scan(&a.ID, &a.RequestID, &a.Level, &a.ApproverID,
		&a.ApproverEmail, &a.ApproverRole, &a.Status, &a.Comment,
		&a.DecidedAt, &a.DueAt, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("approval %s not pending (already decided or missing)", approvalID)
		}
		return nil, err
	}
	return &a, nil
}

// DelegateApproval reassigns a still-pending approval to another
// approver. Immutable once decided (and only while pending).
func (s *Store) DelegateApproval(ctx context.Context, id, toApproverID, toEmail, toRole string) (*Approval, error) {
	if toApproverID == "" && toEmail == "" {
		return nil, fmt.Errorf("delegation target required")
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE workflow_approvals
		SET approver_id = CASE WHEN $2::text != '' THEN $2::uuid ELSE approver_id END,
		    approver_email = CASE WHEN $3::text != '' THEN $3 ELSE approver_email END,
		    approver_role  = CASE WHEN $4::text != '' THEN $4 ELSE approver_role END
		WHERE id = $1 AND status = 'pending'
	`, id, toApproverID, toEmail, toRole)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, fmt.Errorf("approval %s not pending (already decided or missing)", id)
	}
	return s.GetApproval(ctx, id)
}

// GetApproval returns one approval row by id.
func (s *Store) GetApproval(ctx context.Context, id string) (*Approval, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, request_id, level, COALESCE(approver_id::text, ''),
		       COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		       status, COALESCE(comment, ''), decided_at, due_at, created_at
		FROM workflow_approvals WHERE id = $1
	`, id)
	var a Approval
	err := row.Scan(&a.ID, &a.RequestID, &a.Level, &a.ApproverID,
		&a.ApproverEmail, &a.ApproverRole, &a.Status, &a.Comment,
		&a.DecidedAt, &a.DueAt, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ApprovalsPendingFor returns true when the request still has any
// pending (undecided) approval rows.
func (s *Store) ApprovalsPendingFor(ctx context.Context, requestID string) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workflow_approvals
		WHERE request_id = $1 AND status = 'pending'
	`, requestID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListPendingApprovals returns pending approval rows, optionally
// filtered by approver id. Used by the approval inbox.
func (s *Store) ListPendingApprovals(ctx context.Context, approverID string) ([]*Approval, error) {
	q := `
		SELECT a.id, a.request_id, a.level, COALESCE(a.approver_id::text, ''),
		       COALESCE(a.approver_email, ''), COALESCE(a.approver_role, ''),
		       a.status, COALESCE(a.comment, ''), a.decided_at, a.due_at, a.created_at,
		       r.type AS request_type, r.status AS request_status
		FROM workflow_approvals a
		JOIN workflow_requests r ON r.id = a.request_id
		WHERE a.status = 'pending'
	`
	args := []any{}
	if approverID != "" {
		q += ` AND (a.approver_id = $1::uuid OR a.approver_role = (
			SELECT approver_role FROM workflow_approvals WHERE id = a.id AND a.approver_id IS NULL
		))`
		args = append(args, approverID)
	}
	q += ` ORDER BY a.level, a.created_at`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Approval
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.RequestID, &a.Level, &a.ApproverID,
			&a.ApproverEmail, &a.ApproverRole, &a.Status, &a.Comment,
			&a.DecidedAt, &a.DueAt, &a.CreatedAt, &a.RequestType, &a.RequestStatus); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ─── Audit ─────────────────────────────────────────────────────

func (s *Store) AppendAudit(ctx context.Context, requestID, step, actor string, details any) error {
	var detailsJSON []byte
	if details != nil {
		var err error
		detailsJSON, err = json.Marshal(details)
		if err != nil {
			detailsJSON = []byte("{}")
		}
	} else {
		detailsJSON = []byte("{}")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workflow_audit (request_id, step, actor, details, ts)
		VALUES ($1,$2,$3,$4,NOW())
	`, nullString(requestID), step, actor, detailsJSON)
	return err
}

func (s *Store) ListAudit(ctx context.Context, requestID string) ([]*AuditEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, request_id, step, actor, details, ts
		FROM workflow_audit WHERE request_id = $1 ORDER BY ts, id
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuditEntry
	for rows.Next() {
		var a AuditEntry
		var reqID *string
		if err := rows.Scan(&a.ID, &reqID, &a.Step, &a.Actor, &a.Details, &a.TS); err != nil {
			return nil, err
		}
		if reqID != nil {
			a.RequestID = *reqID
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ─── helpers ───────────────────────────────────────────────────

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func ptr[T any](v T) *T {
	return &v
}
