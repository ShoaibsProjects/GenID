package workflow

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── CreateRequest ───────────────────────────────────────────

func TestCreateRequest_NewInsert(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO workflow_requests (
			id, tenant_id, type, status, requester_id, target_id, payload,
			idempotency_key, temporal_workflow_id, temporal_run_id,
			created_at, updated_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (tenant_id, type, idempotency_key)
		WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
		DO NOTHING
		RETURNING id
	`)).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("11111111-1111-1111-1111-111111111111"))

	r := &Request{
		TenantID:    "00000000-0000-0000-0000-000000000001",
		Type:        "access.request.firecall",
		Status:      "pending",
		RequesterID: "00000000-0000-0000-0000-000000000a0a",
		TargetID:    "00000000-0000-0000-0000-000000000b0b",
		Payload:     []byte(`{"resource_id":"prod-db"}`),
		ExpiresAt:   nil,
	}

	created, err := s.CreateRequest(context.Background(), r)
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEmpty(t, r.ID)
	assert.Equal(t, "pending", r.Status)
	assert.False(t, r.CreatedAt.IsZero())

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRequest_IdempotentConflictReturnsExisting(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	insertRe := regexp.QuoteMeta(`
		INSERT INTO workflow_requests (
			id, tenant_id, type, status, requester_id, target_id, payload,
			idempotency_key, temporal_workflow_id, temporal_run_id,
			created_at, updated_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (tenant_id, type, idempotency_key)
		WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
		DO NOTHING
		RETURNING id
	`)
	args13 := func() []any {
		out := make([]any, 13)
		for i := range out {
			out[i] = pgxmock.AnyArg()
		}
		return out
	}

	// First call: insert succeeds.
	mock.ExpectQuery(insertRe).WithArgs(args13()...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("11111111-1111-1111-1111-111111111111"))

	// Second call with same key: insert hits conflict → DO NOTHING (no row).
	mock.ExpectQuery(insertRe).WithArgs(args13()...).WillReturnError(pgx.ErrNoRows)

	// Conflict lookup returns the existing row.
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id FROM workflow_requests
		WHERE tenant_id = $1 AND type = $2 AND idempotency_key = $3
		LIMIT 1
	`)).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("99999999-9999-9999-9999-999999999999"))

	r := &Request{
		TenantID:       "00000000-0000-0000-0000-000000000001",
		Type:           "access.request.firecall",
		Status:         "pending",
		IdempotencyKey: "dedup-key-1",
	}

	created, err := s.CreateRequest(context.Background(), r)
	require.NoError(t, err)
	assert.True(t, created)

	r2 := &Request{
		TenantID:       "00000000-0000-0000-0000-000000000001",
		Type:           "access.request.firecall",
		Status:         "pending",
		IdempotencyKey: "dedup-key-1",
	}
	created, err = s.CreateRequest(context.Background(), r2)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "99999999-9999-9999-9999-999999999999", r2.ID)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── UpdateRequestStatus ────────────────────────────────────

func TestUpdateRequestStatus_Approved(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE workflow_requests
		SET status = $1, updated_at = NOW(),
		    completed_at = CASE WHEN status IN ('executed','denied','failed','cancelled')
		                     THEN NOW() ELSE completed_at END
		WHERE id = $2
	`)).WithArgs("approved", "11111111-1111-1111-1111-111111111111").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = s.UpdateRequestStatus(context.Background(), "11111111-1111-1111-1111-111111111111", "approved")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateRequestStatus_TerminalSetsCompletedAt(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	// Same SQL regardless of status — the CASE handles completion in-DB.
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE workflow_requests
		SET status = $1, updated_at = NOW(),
		    completed_at = CASE WHEN status IN ('executed','denied','failed','cancelled')
		                     THEN NOW() ELSE completed_at END
		WHERE id = $2
	`)).WithArgs("denied", "11111111-1111-1111-1111-111111111111").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = s.UpdateRequestStatus(context.Background(), "11111111-1111-1111-1111-111111111111", "denied")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── SetTemporalIDs ─────────────────────────────────────────

func TestSetTemporalIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE workflow_requests SET temporal_workflow_id=$1, temporal_run_id=$2, updated_at=NOW()
		WHERE id=$3
	`)).WithArgs("wf-1", "run-1", "11111111-1111-1111-1111-111111111111").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = s.SetTemporalIDs(context.Background(), "11111111-1111-1111-1111-111111111111", "wf-1", "run-1")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── AppendAudit ────────────────────────────────────────────

func TestAppendAudit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO workflow_audit (request_id, step, actor, details, ts)
		VALUES ($1,$2,$3,$4,NOW())
	`)).WithArgs("11111111-1111-1111-1111-111111111111", "workflow.requested", "user:test", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = s.AppendAudit(context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"workflow.requested", "user:test",
		map[string]any{"type": "access.request.firecall"})
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── nullString helper ──────────────────────────────────────

func TestNullString(t *testing.T) {
	ns := nullString("hello")
	assert.Equal(t, "hello", ns)

	ns = nullString("")
	assert.Nil(t, ns)
}

// ─── Approval chain ─────────────────────────────────────────

func TestCreateApprovalChain_InsertsSteps(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	// ListApprovals for idempotency check → empty.
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, request_id, level, COALESCE(approver_id::text, ''),
		       COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		       status, COALESCE(comment, ''), decided_at, due_at, created_at
		FROM workflow_approvals WHERE request_id = $1 ORDER BY level
	`)).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "request_id", "level", "approver_id", "approver_email",
			"approver_role", "status", "comment", "decided_at", "due_at", "created_at",
		}))

	// Two CreateApproval inserts.
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO workflow_approvals (id, request_id, level, approver_id, approver_email,
			approver_role, status, due_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`)).
		WithArgs(pgxmock.AnyArg(), "22222222-2222-2222-2222-222222222222", 1,
			nil, "", "resource_owner", "pending", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO workflow_approvals (id, request_id, level, approver_id, approver_email,
			approver_role, status, due_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`)).
		WithArgs(pgxmock.AnyArg(), "22222222-2222-2222-2222-222222222222", 2,
			nil, "", "security_admin", "pending", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	n, err := s.CreateApprovalChain(context.Background(), "22222222-2222-2222-2222-222222222222", []ApprovalStep{
		{Level: 1, ApproverRole: "resource_owner", Mode: "sequential", DueInHours: 24},
		{Level: 2, ApproverRole: "security_admin", Mode: "sequential", DueInHours: 8},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateApprovalChain_IdempotentNoop(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	// Existing row found → no inserts.
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, request_id, level, COALESCE(approver_id::text, ''),
		       COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		       status, COALESCE(comment, ''), decided_at, due_at, created_at
		FROM workflow_approvals WHERE request_id = $1 ORDER BY level
	`)).
		WithArgs("22222222-2222-2222-2222-222222222222").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "request_id", "level", "approver_id", "approver_email",
			"approver_role", "status", "comment", "decided_at", "due_at", "created_at",
		}).AddRow("a", "22222222-2222-2222-2222-222222222222", 1,
			"", "", "resource_owner", "pending", "", nil, nil, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)))

	n, err := s.CreateApprovalChain(context.Background(), "22222222-2222-2222-2222-222222222222", []ApprovalStep{
		{Level: 1, ApproverRole: "resource_owner", Mode: "sequential", DueInHours: 24},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDecideApproval_Approves(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	decidedAt := ptr(time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	createdAt := time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC)

	// Args: decision, comment, approver_id (via nullString), approval_id.
	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE workflow_approvals
		SET status = $1, comment = $2, decided_at = NOW(),
		    approver_id = COALESCE(approver_id, $3::uuid)
		WHERE id = $4 AND status = 'pending'
		RETURNING id, request_id, level, COALESCE(approver_id::text, ''),
		          COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		          status, COALESCE(comment, ''), decided_at, due_at, created_at
	`)).
		WithArgs("approved", "looks fine", "33333333-3333-3333-3333-333333333333",
			"44444444-4444-4444-4444-444444444444").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "request_id", "level", "approver_id", "approver_email",
			"approver_role", "status", "comment", "decided_at", "due_at", "created_at",
		}).AddRow("44444444-4444-4444-4444-444444444444",
			"22222222-2222-2222-2222-222222222222", 1,
			"33333333-3333-3333-3333-333333333333", "owner@observeid.io",
			"resource_owner", "approved", "looks fine",
			decidedAt, nil, createdAt))

	a, err := s.DecideApproval(context.Background(),
		"44444444-4444-4444-4444-444444444444",
		"33333333-3333-3333-3333-333333333333",
		"approved", "looks fine")
	require.NoError(t, err)
	assert.Equal(t, "approved", a.Status)
	assert.Equal(t, "looks fine", a.Comment)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDecideApproval_AlreadyDecidedErrors(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	mock.ExpectQuery(regexp.QuoteMeta(`
		UPDATE workflow_approvals
		SET status = $1, comment = $2, decided_at = NOW(),
		    approver_id = COALESCE(approver_id, $3::uuid)
		WHERE id = $4 AND status = 'pending'
		RETURNING id, request_id, level, COALESCE(approver_id::text, ''),
		          COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		          status, COALESCE(comment, ''), decided_at, due_at, created_at
	`)).
		WithArgs("approved", "", "33333333-3333-3333-3333-333333333333",
			"44444444-4444-4444-4444-444444444444").
		WillReturnError(pgx.ErrNoRows)

	_, err = s.DecideApproval(context.Background(),
		"44444444-4444-4444-4444-444444444444",
		"33333333-3333-3333-3333-333333333333",
		"approved", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not pending")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDelegateApproval_Reassigns(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)
	decidedAt := ptr(time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC))
	createdAt := time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE workflow_approvals
		SET approver_id = CASE WHEN $2::text != '' THEN $2::uuid ELSE approver_id END,
		    approver_email = CASE WHEN $3::text != '' THEN $3 ELSE approver_email END,
		    approver_role  = CASE WHEN $4::text != '' THEN $4 ELSE approver_role END
		WHERE id = $1 AND status = 'pending'
	`)).
		WithArgs("44444444-4444-4444-4444-444444444444",
			"55555555-5555-5555-5555-555555555555", "sec@observeid.io", "security_admin").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, request_id, level, COALESCE(approver_id::text, ''),
		       COALESCE(approver_email, ''), COALESCE(approver_role, ''),
		       status, COALESCE(comment, ''), decided_at, due_at, created_at
		FROM workflow_approvals WHERE id = $1
	`)).
		WithArgs("44444444-4444-4444-4444-444444444444").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "request_id", "level", "approver_id", "approver_email",
			"approver_role", "status", "comment", "decided_at", "due_at", "created_at",
		}).AddRow("44444444-4444-4444-4444-444444444444",
			"22222222-2222-2222-2222-222222222222", 1,
			"55555555-5555-5555-5555-555555555555", "sec@observeid.io",
			"security_admin", "pending", "",
			decidedAt, nil, createdAt))

	a, err := s.DelegateApproval(context.Background(),
		"44444444-4444-4444-4444-444444444444",
		"55555555-5555-5555-5555-555555555555", "sec@observeid.io", "security_admin")
	require.NoError(t, err)
	assert.Equal(t, "55555555-5555-5555-5555-555555555555", a.ApproverID)
	assert.Equal(t, "security_admin", a.ApproverRole)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDelegateApproval_AlreadyDecidedErrors(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	s := NewStore(mock)

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE workflow_approvals
		SET approver_id = CASE WHEN $2::text != '' THEN $2::uuid ELSE approver_id END,
		    approver_email = CASE WHEN $3::text != '' THEN $3 ELSE approver_email END,
		    approver_role  = CASE WHEN $4::text != '' THEN $4 ELSE approver_role END
		WHERE id = $1 AND status = 'pending'
	`)).
		WithArgs("44444444-4444-4444-4444-444444444444",
			"55555555-5555-5555-5555-555555555555", "", "").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	_, err = s.DelegateApproval(context.Background(),
		"44444444-4444-4444-4444-444444444444",
		"55555555-5555-5555-5555-555555555555", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not pending")
	require.NoError(t, mock.ExpectationsWereMet())
}
