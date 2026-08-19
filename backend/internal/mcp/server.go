package mcp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools wires the five MCP tools onto the server. Each handler
// resolves the session (per-request for SSE, bootstrap for stdio) and
// audits the call via AuditCall.
func RegisterTools(s *server.MCPServer, d *Deps) {
	tools := []struct {
		name string
		desc string
		opts []mcp.ToolOption
		fn   func(context.Context, *Deps, *Session, mcp.CallToolRequest) *mcp.CallToolResult
	}{
		{
			name: "query_identity",
			desc: "Resolve an identity (UUID or email) and optionally fetch its entitlements graph and risk score.",
			opts: []mcp.ToolOption{
				mcp.WithString("identity_id", mcp.Required(), mcp.Description("Identity UUID or email")),
				mcp.WithBoolean("include_entitlements", mcp.Description("Include Neo4j entitlements graph (default false)")),
				mcp.WithBoolean("include_risk", mcp.Description("Include computed risk score + factors (default false)")),
			},
			fn: handleQueryIdentity,
		},
		{
			name: "request_access",
			desc: "Request access for an identity to a resource. Starts the same conditional-access workflow as the API, including approval routing.",
			opts: []mcp.ToolOption{
				mcp.WithString("identity_id", mcp.Required(), mcp.Description("Identity UUID or email requesting access")),
				mcp.WithString("resource_id", mcp.Required(), mcp.Description("Resource UUID to request access to")),
				mcp.WithString("duration", mcp.Description("Duration: 2h (default), 1d, permanent, or N hours")),
				mcp.WithString("reason", mcp.Required(), mcp.Description("Business justification")),
			},
			fn: handleRequestAccess,
		},
		{
			name: "check_risk",
			desc: "Compute the current risk score and band for an identity, with contributing factors.",
			opts: []mcp.ToolOption{
				mcp.WithString("identity_id", mcp.Required(), mcp.Description("Identity UUID or email")),
			},
			fn: handleCheckRisk,
		},
		{
			name: "explain_access",
			desc: "Explain WHY an identity can or cannot access a resource: entitlements path, Cedar policy decision and risk context.",
			opts: []mcp.ToolOption{
				mcp.WithString("identity_id", mcp.Required(), mcp.Description("Identity UUID or email")),
				mcp.WithString("resource_id", mcp.Required(), mcp.Description("Resource UUID")),
			},
			fn: handleExplainAccess,
		},
		{
			name: "list_approvals",
			desc: "List approval requests, optionally filtered by identity and status.",
			opts: []mcp.ToolOption{
				mcp.WithString("identity_id", mcp.Description("Filter by target or requester identity UUID")),
				mcp.WithString("status", mcp.Description("Filter by approval status (pending, approved, denied, expired)")),
			},
			fn: handleListApprovals,
		},
	}

	for _, t := range tools {
		tool := mcp.NewTool(t.name, append([]mcp.ToolOption{mcp.WithDescription(t.desc)}, t.opts...)...)
		fn := t.fn
		s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return fn(ctx, d, session(ctx, d), req), nil
		})
	}
}

// NewMCPServer builds the fully-registered GenID MCP server.
func NewMCPServer(d *Deps) *server.MCPServer {
	s := server.NewMCPServer(
		"genid-mcp",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)
	RegisterTools(s, d)
	RegisterResources(s, d)
	return s
}

// ServeStdio runs the server over stdin/stdout. The process must have been
// started with a valid MCP_API_KEY (validated at boot).
func ServeStdio(d *Deps) error {
	if err := ValidateStdioAPIKey(d); err != nil {
		return err
	}
	return server.NewStdioServer(NewMCPServer(d)).Listen(context.Background(), os.Stdin, os.Stdout)
}

// ServeSSE runs the server over HTTP+SSE behind API-key auth.
func ServeSSE(ctx context.Context, d *Deps, addr string) error {
	sseServer := server.NewSSEServer(NewMCPServer(d), server.WithBaseURL("http://"+addr))
	handler := APIKeyMiddleware(d.APIKeys, sseServer)

	srv := &http.Server{Addr: addr, Handler: handler}
	log.Printf("[mcp] SSE server listening on %s", addr)
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	}
}
