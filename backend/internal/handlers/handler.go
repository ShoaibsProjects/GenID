package handlers

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/observeid/genid/internal/middleware"
	"github.com/observeid/genid/internal/services"
)

// Handler is the HTTP layer. It embeds *services.Service so every
// dependency accessor and business method is promoted; it adds no state.
type Handler struct {
	*services.Service
}

// NewHandler wires handlers to the business-logic service.
func NewHandler(svc *services.Service) *Handler {
	return &Handler{Service: svc}
}

// DB returns the tenant-scoped transaction from context (set by TenantMiddleware),
// or falls back to the raw connection pool. Every handler should use h.DB(r.Context())
// instead of h.Pool() for Postgres queries so RLS filtering applies.
func (h *Handler) DB(ctx context.Context) middleware.DBQuerier {
	return middleware.TenantDB(ctx, h.Pool())
}

// RawPool returns the raw pgxpool for cases that genuinely need it
// (starting new transactions, health checks, migrations).
func (h *Handler) RawPool() *pgxpool.Pool {
	return h.Pool()
}
