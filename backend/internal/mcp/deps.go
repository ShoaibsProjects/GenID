package mcp

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/audit"
	cedar "github.com/observeid/genid/internal/cedar"
	"github.com/observeid/genid/internal/middleware"
	"github.com/observeid/genid/internal/workflow"
	"go.temporal.io/sdk/client"
	tlog "go.temporal.io/sdk/log"
)

const (
	DefaultTenant = "00000000-0000-0000-0000-000000000001"
	DefaultTaskQ  = "critical-offboarding"
)

// Scope constants enforced by the MCP auth layer. Tool calls that mutate
// state (request_access) need mcp:write; read-only tools need mcp:read.
const (
	ScopeMCPRead  = "mcp:read"
	ScopeMCPWrite = "mcp:write"
)

// Deps carries every store the MCP tools read from or write to. The
// mcp-server binary wires these exactly like the identity-service.
type Deps struct {
	Pool           *pgxpool.Pool
	Neo4j          neo4j.DriverWithContext
	Cedar          *cedar.CedarEngine
	Workflows      *workflow.Store
	Audit          *audit.Chain
	APIKeys        middleware.APIKeyStore
	Temporal       client.Client
	TenantID       string
	TaskQueue      string
	MCPAPIKey      string
	APIKeyName     string
	DefaultSession *Session
}

// NewDeps connects to Postgres, Neo4j and Temporal. A failed connection is
// fatal: an MCP server with a half-open graph is worse than no server.
func NewDeps(ctx context.Context, databaseURL, neo4jURI, neo4jUser, neo4jPassword, temporalHost, temporalNamespace, apiKey, apiKeyName string) (*Deps, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPassword, ""))
	if err != nil {
		return nil, err
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, err
	}

	tc, err := client.Dial(client.Options{
		HostPort:  temporalHost,
		Namespace: temporalNamespace,
		Logger:    tlog.NewStructuredLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
	})
	if err != nil {
		return nil, err
	}

	return &Deps{
		Pool:       pool,
		Neo4j:      driver,
		Cedar:      cedar.NewCedarEngine(pool),
		Workflows:  workflow.NewStore(pool),
		Audit:      audit.NewChain(pool),
		APIKeys:    middleware.NewPGAPIKeyStore(pool),
		Temporal:   tc,
		TenantID:   DefaultTenant,
		TaskQueue:  DefaultTaskQ,
		MCPAPIKey:  apiKey,
		APIKeyName: apiKeyName,
	}, nil
}

// Close releases the underlying connections.
func (d *Deps) Close(ctx context.Context) {
	d.Pool.Close()
	_ = d.Neo4j.Close(ctx)
	d.Temporal.Close()
}

// now is a tiny indirection so tests can pin the clock if needed.
var now = time.Now
