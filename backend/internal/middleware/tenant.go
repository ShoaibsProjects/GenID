package middleware

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/observeid/genid/internal/vault"
)

const tenantTxKey contextKey = "tenant_tx"

// TenantMiddleware extracts the tenant ID from the X-Tenant-ID header
// (preferred) or JWT context, then wraps each request in a PostgreSQL
// transaction with RLS tenant context set. Handlers retrieve the
// transaction via h.DB(ctx). The transaction is committed on success;
// rolled back on error.
func TenantMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				tenantID = TenantIDFromContext(r.Context())
			}
			// Default to the main tenant if not specified
			if tenantID == "" {
				tenantID = "00000000-0000-0000-0000-000000000001"
			}

			tx, err := pool.Begin(r.Context())
			if err != nil {
				http.Error(w, `{"error":"failed to start tenant transaction"}`, http.StatusInternalServerError)
				return
			}
			if _, err := tx.Exec(r.Context(), "SET app.current_tenant = '"+tenantID+"'"); err != nil {
				tx.Rollback(r.Context())
				http.Error(w, `{"error":"failed to set tenant context"}`, http.StatusInternalServerError)
				return
			}

ctx := context.WithValue(r.Context(), tenantTxKey, tx)
		ctx = context.WithValue(ctx, vault.TenantCtxKey{}, tenantID)
		ctx = context.WithValue(ctx, vault.TxContextKey{}, tx)
		r = r.WithContext(ctx)

			rw := &tenantResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)

			if rw.statusCode >= 200 && rw.statusCode < 400 {
				tx.Commit(r.Context())
			} else {
				tx.Rollback(r.Context())
			}
		})
	}
}

type tenantResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *tenantResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// TenantTxFromContext returns the tenant-scoped transaction stored by TenantMiddleware.
func TenantTxFromContext(ctx context.Context) pgx.Tx {
	if tx, ok := ctx.Value(tenantTxKey).(pgx.Tx); ok {
		return tx
	}
	return nil
}

// TenantDB returns the tenant-scoped transaction from context if present,
// otherwise falls back to the raw connection pool.
func TenantDB(ctx context.Context, pool *pgxpool.Pool) DBQuerier {
	if tx := TenantTxFromContext(ctx); tx != nil {
		return tx
	}
	return pool
}

// DBQuerier is the common interface shared by pgx.Tx and pgxpool.Pool.
type DBQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
