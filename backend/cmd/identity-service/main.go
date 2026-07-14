package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/observeid/identity-platform/internal/activities"
	"github.com/observeid/identity-platform/internal/audit"
	"github.com/observeid/identity-platform/internal/graphql"
	"github.com/observeid/identity-platform/internal/middleware"
	"github.com/observeid/identity-platform/internal/service"
	"github.com/observeid/identity-platform/internal/workflow"
	"github.com/observeid/identity-platform/pkg/telemetry"
)

func main() {
	// ─── Load .env if present ────────────────────────────
	if data, err := os.ReadFile(".env"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				if os.Getenv(key) == "" {
					os.Setenv(key, val)
				}
			}
		}
	}

	// ─── Initialize Structured Logger ─────────────────────
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Str("service", "observeid-identity").
		Logger()

	log.Info().Msg("═══════════════════════════════════════════")
	log.Info().Msg("  ObserveID Reimagined Identity Service Starting")
	log.Info().Msg("  The Identity Fabric Engine")
	log.Info().Msg("═══════════════════════════════════════════")

	// ─── Load Configuration ───────────────────────────────
	cfg := loadConfig()

	// ─── Initialize OpenTelemetry ─────────────────────────
	shutdown := initTelemetry(cfg)
	defer shutdown()

	// ─── Initialize PostgreSQL ────────────────────────────
	pgPool, err := service.NewPostgresPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	defer pgPool.Close()
	log.Info().Msg("PostgreSQL connected")

	// ─── Initialize Neo4j ─────────────────────────────────
	neo4jDriver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Neo4j")
	}
	defer neo4jDriver.Close(context.Background())
	log.Info().Msg("Neo4j connected")

	// ─── Initialize Redis ─────────────────────────────────
	redisOpts := &redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	}
	if cfg.RedisTLS {
		redisOpts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	rdb := redis.NewClient(redisOpts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer rdb.Close()
	log.Info().Msg("Redis connected")

	// ─── Initialize Temporal Client ───────────────────────
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalHost,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Temporal")
	}
	defer temporalClient.Close()
	log.Info().Msg("Temporal connected")

	// ─── Initialize Services ──────────────────────────────
	svc := service.NewIdentityService(pgPool, neo4jDriver, rdb, temporalClient)
	auditLogStore := svc.AuditStore()

	// Load persisted connectors from PostgreSQL on startup
	if err := svc.LoadConnectors(context.Background()); err != nil {
		log.Warn().Err(err).Msg("Failed to load persisted connectors")
	}

	// ─── Start Temporal Worker ────────────────────────────
	w := worker.New(temporalClient, cfg.TemporalNamespace, worker.Options{
		MaxConcurrentActivityExecutionSize: 500,
		MaxConcurrentWorkflowTaskExecutionSize: 500,
		// StickyCacheSize: deprecated in newer SDK
	})

	w.RegisterWorkflow(workflow.OffboardIdentityWorkflow)
	w.RegisterWorkflow(workflow.OnboardIdentityWorkflow)
	w.RegisterWorkflow(workflow.GrantAccessWorkflow)
	w.RegisterWorkflow(workflow.RevokeAccessWorkflow)
	w.RegisterWorkflow(workflow.JustInTimeAccessWorkflow)
	w.RegisterWorkflow(workflow.AgentAnomalyDetectionWorkflow)
	w.RegisterWorkflow(workflow.DetectSoDViolationsWorkflow)
	w.RegisterWorkflow(workflow.CascadeRevokeWorkflow)
	w.RegisterWorkflow(workflow.RevokeAccessChildWorkflow)

	act := activities.NewActivityService(pgPool, neo4jDriver, rdb, temporalClient)
	w.RegisterActivity(act)

	if err := w.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start Temporal worker")
	}
	defer w.Stop()
	log.Info().Msg("Temporal worker started")

	// ─── Initialize Security Middleware ────────────────────
	rateLimiter := middleware.NewRateLimiter(100, 200) // 100 req/s, burst 200
	apiKeyAuth := middleware.NewAPIKeyAuth(loadAPIKeys(), "/health", "/ready", "/healthz", "/")
	requestValidation := middleware.NewRequestValidation()

	// ─── Start HTTP/gRPC Server ───────────────────────────
	r := mux.NewRouter()
	r.Use(securityHeadersMiddleware)
	r.Use(corsMiddleware(cfg.CORSOrigin))
	r.Use(otelhttp.NewMiddleware("observeid-api"))
	r.Use(rateLimiter.Middleware)
	r.Use(requestValidation.Middleware)
	r.Use(apiKeyAuth.Middleware)
	r.Use(audit.LoggingMiddleware(auditLogStore))

	// Serve static frontend from the frontend/out directory
	frontendDir := getEnv("FRONTEND_DIR", "")
	if frontendDir == "" {
		// Try common relative paths based on where the binary is run
		candidates := []string{"frontend/out", "../frontend/out", "./frontend/out"}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && fi.IsDir() {
				frontendDir = c
				break
			}
		}
	}
	if frontendDir != "" {
		fs := http.FileServer(http.Dir(frontendDir))
		r.PathPrefix("/_next/").Handler(fs)

		// Next.js static export creates flat .html files (dashboard.html, identities.html, etc.)
		// Map each frontend route to its .html file
		frontendPages := []string{"dashboard", "identities", "agents", "connectors", "groups",
			"access", "policies", "audit", "certifications", "sod", "vault", "settings"}
		for _, page := range frontendPages {
			p := page // capture
			r.HandleFunc("/"+p, func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, frontendDir+"/"+p+".html")
			})
		}

		// Root serves index.html
		r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, frontendDir+"/index.html")
		})

		log.Info().Msg("Frontend static files serving from " + frontendDir)
	} else {
		log.Warn().Msg("Frontend static directory not found (checked: frontend/out, ../frontend/out), serving API-only")
		// Root — API documentation landing page
		r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ObserveID — Identity Fabric Engine</title>
<style>
  :root {
    --bg: #09090B; --surface: #111116; --border: #1E1E24;
    --text: #FAFAFA; --muted: #A1A1AA; --dim: #52525B;
    --accent: #3B82F6; --accent-dim: rgba(59,130,246,0.12);
    --green: #22C55E; --green-dim: rgba(34,197,94,0.12);
    --amber: #F59E0B; --amber-dim: rgba(245,158,11,0.12);
    --red: #EF4444;
  }
  * { margin:0; padding:0; box-sizing:border-box; }
  body {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    background: var(--bg); color: var(--text); line-height: 1.6;
    min-height: 100vh;
  }
  body::before {
    content: ''; position: fixed; inset: 0; z-index: -1;
    background-image:
      linear-gradient(rgba(255,255,255,0.02) 1px, transparent 1px),
      linear-gradient(90deg, rgba(255,255,255,0.02) 1px, transparent 1px);
    background-size: 48px 48px;
  }
  .container { max-width: 960px; margin: 0 auto; padding: 3rem 1.5rem; }

  /* Header */
  .header { text-align: center; margin-bottom: 3rem; }
  .logo {
    width: 56px; height: 56px; border-radius: 14px;
    background: linear-gradient(135deg, var(--accent), #2563EB);
    display: inline-flex; align-items: center; justify-content: center;
    margin-bottom: 1.25rem; box-shadow: 0 0 40px rgba(59,130,246,0.15);
  }
  .logo svg { width: 28px; height: 28px; color: white; }
  h1 { font-size: 1.75rem; font-weight: 700; letter-spacing: -0.02em; margin-bottom: 0.5rem; }
  .subtitle { color: var(--muted); font-size: 0.95rem; max-width: 520px; margin: 0 auto; }

  /* Status pill */
  .status-row { display: flex; justify-content: center; gap: 1.5rem; margin: 1.5rem 0 2.5rem; flex-wrap: wrap; }
  .status-pill {
    display: inline-flex; align-items: center; gap: 0.5rem;
    padding: 0.4rem 1rem; border-radius: 100px; font-size: 0.8rem; font-weight: 500;
    border: 1px solid var(--border); background: var(--surface);
  }
  .dot { width: 7px; height: 7px; border-radius: 50%; }
  .dot-green { background: var(--green); box-shadow: 0 0 8px var(--green); }
  .dot-amber { background: var(--amber); box-shadow: 0 0 8px var(--amber); }

  /* Grid */
  .section-title {
    font-size: 0.7rem; font-weight: 600; text-transform: uppercase;
    letter-spacing: 0.08em; color: var(--dim); margin-bottom: 0.75rem;
  }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 0.75rem; margin-bottom: 2.5rem; }

  /* Cards */
  .card {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 10px; padding: 1rem 1.15rem; transition: border-color 0.2s;
  }
  .card:hover { border-color: var(--accent); }
  .card-header { display: flex; align-items: center; gap: 0.6rem; margin-bottom: 0.4rem; }
  .card-icon {
    width: 32px; height: 32px; border-radius: 8px; display: flex;
    align-items: center; justify-content: center; flex-shrink: 0;
  }
  .card-icon svg { width: 16px; height: 16px; }
  .card-title { font-size: 0.85rem; font-weight: 600; }
  .card-desc { font-size: 0.78rem; color: var(--muted); line-height: 1.5; }

  /* Endpoint list */
  .ep-list { list-style: none; }
  .ep-item {
    display: flex; align-items: center; gap: 0.75rem;
    padding: 0.6rem 0; border-bottom: 1px solid var(--border);
    font-size: 0.82rem;
  }
  .ep-item:last-child { border-bottom: none; }
  .method {
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 0.65rem; font-weight: 700; padding: 0.2rem 0.5rem;
    border-radius: 4px; min-width: 48px; text-align: center;
    text-transform: uppercase; letter-spacing: 0.03em;
  }
  .method-get { background: var(--green-dim); color: var(--green); }
  .method-post { background: var(--accent-dim); color: var(--accent); }
  .method-put { background: var(--amber-dim); color: var(--amber); }
  .method-delete { background: rgba(239,68,68,0.12); color: var(--red); }
  .method-query { background: rgba(168,85,247,0.12); color: #A855F7; }
  .ep-path { font-family: 'JetBrains Mono', 'Fira Code', monospace; color: var(--text); font-size: 0.78rem; }
  .ep-desc { color: var(--dim); margin-left: auto; font-size: 0.72rem; white-space: nowrap; }

  /* Footer */
  .footer {
    margin-top: 3rem; padding-top: 1.5rem; border-top: 1px solid var(--border);
    display: flex; justify-content: space-between; align-items: center;
    font-size: 0.72rem; color: var(--dim); flex-wrap: wrap; gap: 1rem;
  }
  .footer a { color: var(--accent); text-decoration: none; }
  .footer a:hover { text-decoration: underline; }

  /* Animations */
  @keyframes pulse { 0%,100% { opacity:1; } 50% { opacity:0.5; } }
  .dot-green { animation: pulse 2s ease-in-out infinite; }

  @media (max-width: 640px) {
    .container { padding: 2rem 1rem; }
    h1 { font-size: 1.4rem; }
    .grid { grid-template-columns: 1fr; }
    .ep-desc { display: none; }
  }
</style>
</head>
<body>
<div class="container">

  <div class="header">
    <div class="logo">
      <svg fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/>
      </svg>
    </div>
    <h1>ObserveID Identity Fabric Engine</h1>
    <p class="subtitle">Event-Driven, AI-Native Identity Governance Platform. Real-time access control for humans, AI agents, and machines.</p>
  </div>

  <div class="status-row">
    <span class="status-pill"><span class="dot dot-green"></span> API Operational</span>
    <span class="status-pill" style="color:var(--muted)">v1.0.0</span>
    <span class="status-pill" style="color:var(--muted)">SCIM 2.0</span>
    <span class="status-pill" style="color:var(--muted)">RFC 10008</span>
  </div>

  <!-- Core Capabilities -->
  <p class="section-title">Core Capabilities</p>
  <div class="grid">
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:var(--accent-dim)">
          <svg fill="none" viewBox="0 0 24 24" stroke="var(--accent)" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
        </div>
        <span class="card-title">Identity Fabric</span>
      </div>
      <p class="card-desc">Unified identity graph across humans, service accounts, and AI agents with real-time relationship mapping.</p>
    </div>
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:var(--green-dim)">
          <svg fill="none" viewBox="0 0 24 24" stroke="var(--green)" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
        </div>
        <span class="card-title">Access Governance</span>
      </div>
      <p class="card-desc">Policy-as-code with Cedar evaluation, SoD enforcement, and blast radius analysis for least-privilege.</p>
    </div>
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:rgba(168,85,247,0.12)">
          <svg fill="none" viewBox="0 0 24 24" stroke="#A855F7" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
        </div>
        <span class="card-title">Durable Execution</span>
      </div>
      <p class="card-desc">Temporal-powered workflows for automated onboarding, offboarding, and just-in-time access with full audit trails.</p>
    </div>
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:var(--amber-dim)">
          <svg fill="none" viewBox="0 0 24 24" stroke="var(--amber)" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/></svg>
        </div>
        <span class="card-title">Agent Identity</span>
      </div>
      <p class="card-desc">First-class NHI (Non-Human Identity) management with kill-switch delegation and agent card attestation.</p>
    </div>
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:rgba(239,68,68,0.12)">
          <svg fill="none" viewBox="0 0 24 24" stroke="var(--red)" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
        </div>
        <span class="card-title">Secrets Vault</span>
      </div>
      <p class="card-desc">Encrypted credential storage with AES-256-GCM, connector-aware secret rotation, and audit logging.</p>
    </div>
    <div class="card">
      <div class="card-header">
        <div class="card-icon" style="background:var(--accent-dim)">
          <svg fill="none" viewBox="0 0 24 24" stroke="var(--accent)" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4"/></svg>
        </div>
        <span class="card-title">Graph-Powered Analytics</span>
      </div>
      <p class="card-desc">Neo4j identity graph with Cypher queries, real-time SoD detection, and cascade impact analysis.</p>
    </div>
  </div>

  <!-- API Endpoints -->
  <p class="section-title">API Reference</p>
  <div class="card" style="margin-bottom: 2.5rem">
    <ul class="ep-list">
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/health</span><span class="ep-desc">Liveness probe</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/healthz</span><span class="ep-desc">Full dependency check</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/ready</span><span class="ep-desc">Readiness probe</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/metrics</span><span class="ep-desc">Prometheus metrics</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/graphql</span><span class="ep-desc">GraphQL API</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/scim/v2/Users</span><span class="ep-desc">SCIM 2.0 user listing</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/scim/v2/Users</span><span class="ep-desc">SCIM 2.0 provisioning</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/identities</span><span class="ep-desc">List identities</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/identities/{id}</span><span class="ep-desc">Get identity details</span></li>
      <li class="ep-item"><span class="method method-query">QUERY</span><span class="ep-path">/api/v1/access/check</span><span class="ep-desc">Real-time access evaluation</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/access/grant</span><span class="ep-desc">Grant access (Temporal)</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/access/revoke</span><span class="ep-desc">Revoke access (Temporal)</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/access/jit</span><span class="ep-desc">Just-in-time access</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/agents</span><span class="ep-desc">List AI agents / NHIs</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/agents</span><span class="ep-desc">Register agent</span></li>
      <li class="ep-item"><span class="method method-query">QUERY</span><span class="ep-path">/api/v1/copilot/query</span><span class="ep-desc">AI copilot (GraphRAG)</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/connectors</span><span class="ep-desc">List connectors</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/connectors</span><span class="ep-desc">Create connector</span></li>
      <li class="ep-item"><span class="method method-query">QUERY</span><span class="ep-path">/api/v1/connectors/test</span><span class="ep-desc">Test connection</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/connectors/{id}/sync</span><span class="ep-desc">Sync connector data</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/groups</span><span class="ep-desc">List groups / roles</span></li>
      <li class="ep-item"><span class="method method-post">POST</span><span class="ep-path">/api/v1/lcm</span><span class="ep-desc">Lifecycle management</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/vault/secrets</span><span class="ep-desc">List encrypted secrets</span></li>
      <li class="ep-item"><span class="method method-get">GET</span><span class="ep-path">/api/v1/audit/logs</span><span class="ep-desc">Audit trail</span></li>
    </ul>
  </div>

  <div class="footer">
    <span>ObserveID Reimagined &mdash; Identity Fabric Engine</span>
    <span>
      <a href="/health">Health</a> &middot;
      <a href="/ready">Ready</a> &middot;
      <a href="/metrics">Metrics</a> &middot;
      <a href="/graphql">GraphQL</a>
    </span>
  </div>

</div>
</body>
</html>`)
		}).Methods("GET")
	}

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"observeid-identity","version":"1.0.0"}`)
	}).Methods("GET")

	r.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}

		if err := rdb.Ping(r.Context()).Err(); err != nil {
			checks["redis"] = "down"
		} else {
			checks["redis"] = "ok"
		}

		if err := pgPool.Ping(r.Context()); err != nil {
			checks["postgres"] = "down"
		} else {
			checks["postgres"] = "ok"
		}

		if err := neo4jDriver.VerifyConnectivity(r.Context()); err != nil {
			checks["neo4j"] = "down"
		} else {
			checks["neo4j"] = "ok"
		}

		for _, status := range checks {
			if status == "down" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]any{"status": "unavailable", "checks": checks})
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"status": "ready", "checks": checks})
	}).Methods("GET")

	// Healthz — full dependency check (Fly.io / cloud load balancer probe)
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}

		if err := pgPool.Ping(r.Context()); err != nil {
			checks["postgres"] = "down"
		} else {
			checks["postgres"] = "ok"
		}

		if err := neo4jDriver.VerifyConnectivity(r.Context()); err != nil {
			checks["neo4j"] = "down"
		} else {
			checks["neo4j"] = "ok"
		}

		if err := rdb.Ping(r.Context()).Err(); err != nil {
			checks["redis"] = "down"
		} else {
			checks["redis"] = "ok"
		}

		temporalCtx, temporalCancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer temporalCancel()
		if _, err := temporalClient.CheckHealth(temporalCtx, &client.CheckHealthRequest{}); err != nil {
			checks["temporal"] = "down"
		} else {
			checks["temporal"] = "ok"
		}

		for _, status := range checks {
			if status == "down" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]any{"status": "unavailable", "checks": checks})
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"status": "healthy", "checks": checks})
	}).Methods("GET")

	// GraphQL API
	gqlSrv := handler.NewDefaultServer(
		graphql.NewExecutableSchema(graphql.Config{
			Resolvers: &graphql.Resolver{Svc: svc},
		}),
	)
	r.Handle("/graphql", gqlSrv).Methods("POST")

	// SCIM endpoints
	scim := r.PathPrefix("/scim/v2").Subrouter()
	scim.HandleFunc("/Users", svc.ScimListUsers).Methods("GET")
	scim.HandleFunc("/Users", svc.ScimCreateUser).Methods("POST")
	scim.HandleFunc("/Users/{id}", svc.ScimGetUser).Methods("GET")
	scim.HandleFunc("/Users/{id}", svc.ScimUpdateUser).Methods("PUT")
	scim.HandleFunc("/Users/{id}", svc.ScimPatchUser).Methods("PATCH")
	scim.HandleFunc("/Users/{id}", svc.ScimDeleteUser).Methods("DELETE")

	// Identity API
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/identities", svc.ListIdentities).Methods("GET")
	api.HandleFunc("/identities/{id}", svc.GetIdentity).Methods("GET")
	api.HandleFunc("/identities/{id}/entitlements", svc.GetIdentityEntitlements).Methods("GET")
	api.HandleFunc("/identities/{id}/blast-radius", svc.GetBlastRadius).Methods("GET")

	// NHI/Agent API
	api.HandleFunc("/agents", svc.ListAgents).Methods("GET")
	api.HandleFunc("/agents", svc.RegisterAgent).Methods("POST")
	api.HandleFunc("/agents/{id}", svc.GetAgent).Methods("GET")
	api.HandleFunc("/agents/{id}/kill-switch", svc.AgentKillSwitch).Methods("POST")
	api.HandleFunc("/agents/{id}/delegate", svc.DelegateAgent).Methods("POST")
	api.HandleFunc("/agents/{id}/card", svc.GetAgentCard).Methods("GET")

	// Access API
	api.HandleFunc("/access/check", svc.CheckAccess).Methods("QUERY", "POST")
	api.HandleFunc("/access/grant", svc.GrantAccess).Methods("POST")
	api.HandleFunc("/access/revoke", svc.RevokeAccess).Methods("POST")
	api.HandleFunc("/access/jit", svc.JustInTimeAccess).Methods("POST")

	// AI Copilot API
	api.HandleFunc("/copilot/query", svc.CopilotQuery).Methods("QUERY", "POST")

	// CAEP API
	api.HandleFunc("/caep/events", svc.ListCAEPEvents).Methods("GET")
	api.HandleFunc("/caep/broadcast", svc.BroadcastCAEP).Methods("POST")

	// ─── Connector Management ───────────────────────
	api.HandleFunc("/connectors", svc.ListConnectors).Methods("GET")
	api.HandleFunc("/connectors", svc.CreateConnector).Methods("POST")
	api.HandleFunc("/connectors/test", svc.TestConnectorConnection).Methods("QUERY", "POST")
	api.HandleFunc("/connectors/{id}", svc.GetConnector).Methods("GET")
	api.HandleFunc("/connectors/{id}", svc.DeleteConnector).Methods("DELETE")
	api.HandleFunc("/connectors/{id}/connect", svc.ConnectConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/disconnect", svc.DisconnectConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/test", svc.TestExistingConnector).Methods("QUERY", "POST")
	api.HandleFunc("/connectors/{id}/sync", svc.SyncConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/sync-delta", svc.SyncConnectorDelta).Methods("POST")
	api.HandleFunc("/connectors/{id}/users", svc.GetConnectorUsers).Methods("GET")
	api.HandleFunc("/connectors/{id}/identities", svc.GetConnectorIdentities).Methods("GET")
	api.HandleFunc("/connectors/{id}/schema", svc.GetConnectorSchema).Methods("GET")
	api.HandleFunc("/connectors/{id}/health", svc.GetConnectorHealth).Methods("GET")
	api.HandleFunc("/connectors/{id}/groups", svc.GetConnectorGroups).Methods("GET")
	api.HandleFunc("/connectors/{id}/entitlements", svc.GetConnectorEntitlements).Methods("GET")
	api.HandleFunc("/connectors/{id}/resources", svc.GetConnectorResources).Methods("GET")
	api.HandleFunc("/connectors/{id}/full-sync", svc.FullSyncConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/sync-groups", svc.SyncConnectorGroups).Methods("POST")
	api.HandleFunc("/connectors/{id}/sync-entitlements", svc.SyncConnectorEntitlements).Methods("POST")
	api.HandleFunc("/connectors/{id}/sync-resources", svc.SyncConnectorResources).Methods("POST")
	api.HandleFunc("/connectors/csv/upload", svc.CSVUpload).Methods("POST")

	// ─── IAM Lifecycle Management (LCM) ────────────
	api.HandleFunc("/lcm", svc.ExecuteLCM).Methods("POST")
	api.HandleFunc("/lcm/history", svc.GetLCMHistory).Methods("GET")

	// ─── Identity CRUD ─────────────────────────────
	api.HandleFunc("/identities", svc.CreateIdentityRecord).Methods("POST")
	api.HandleFunc("/identities/bulk", svc.BulkImportIdentities).Methods("POST")
	api.HandleFunc("/identities/{id}", svc.UpdateIdentityRecord).Methods("PATCH")
	api.HandleFunc("/identities/{id}", svc.DeleteIdentityRecord).Methods("DELETE")

	// ─── Vault / Secrets ────────────────────────────
	api.HandleFunc("/vault/secrets", svc.ListSecrets).Methods("GET")
	api.HandleFunc("/vault/secrets", svc.StoreSecret).Methods("POST")
	api.HandleFunc("/vault/secrets/{id}", svc.RetrieveSecret).Methods("GET")
	api.HandleFunc("/vault/secrets/{id}", svc.DeleteSecret).Methods("DELETE")

	// ─── Role / Group Management ───────────────────
	api.HandleFunc("/groups", svc.ListGroups).Methods("GET")
	api.HandleFunc("/groups", svc.CreateGroup).Methods("POST")
	api.HandleFunc("/groups/{id}", svc.DeleteGroup).Methods("DELETE")
	api.HandleFunc("/roles/assign", svc.AssignRole).Methods("POST")
	api.HandleFunc("/roles/remove", svc.RemoveRole).Methods("POST")

	// ─── Audit / Access Logs ──────────────────────
	api.HandleFunc("/audit/logs", svc.ListAuditLogs).Methods("GET")
	api.HandleFunc("/audit/logs/{id}", svc.GetAuditLog).Methods("GET")
	api.HandleFunc("/audit/stats", svc.GetAuditLogStats).Methods("GET")

	// Metrics
	r.Handle("/metrics", telemetry.MetricsHandler()).Methods("GET")

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    262144, // 256 KB
	}

	// Graceful Shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("Shutting down gracefully...")
		// Save vault secrets to disk
		if err := svc.SaveVault(); err != nil {
			log.Warn().Err(err).Msg("Failed to save vault")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Info().Str("addr", srv.Addr).Msg("HTTP server listening")

	certFile := getEnv("TLS_CERT_FILE", "")
	keyFile := getEnv("TLS_KEY_FILE", "")
	var srvErr error
	if certFile != "" && keyFile != "" {
		log.Info().Msg("TLS enabled")
		srvErr = srv.ListenAndServeTLS(certFile, keyFile)
	} else {
		srvErr = srv.ListenAndServe()
	}
	if srvErr != nil && srvErr != http.ErrServerClosed {
		log.Fatal().Err(srvErr).Msg("Server failed")
	}
	log.Info().Msg("Server stopped")
}

// ─── Security Headers Middleware ────────────────────────────
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// ─── CORS Middleware ───────────────────────────────────────
func corsMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, QUERY, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if allowedOrigin != "" && allowedOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type Config struct {
	DatabaseURL      string
	Neo4jURI         string
	Neo4jUser        string
	Neo4jPassword    string
	RedisAddr        string
	RedisPassword    string
	RedisTLS         bool
	TemporalHost     string
	TemporalNamespace string
	CORSOrigin       string
	QdrantAddr       string
}

func loadConfig() *Config {
	return &Config{
		DatabaseURL:      getEnv("DATABASE_URL", "postgresql://observeid:observeid@localhost:5432/observeid?sslmode=disable"),
		Neo4jURI:         getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:        getEnv("NEO4J_USER", "neo4j"),
		Neo4jPassword:    getEnv("NEO4J_PASSWORD", ""),
		RedisAddr:        getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:    getEnv("REDIS_PASSWORD", ""),
		RedisTLS:         getEnv("REDIS_TLS", "false") == "true",
		TemporalHost:     getEnv("TEMPORAL_HOST", "localhost:7233"),
		TemporalNamespace: getEnv("TEMPORAL_NAMESPACE", "critical-offboarding"),
		CORSOrigin:       getEnv("CORS_ORIGIN", ""),
		QdrantAddr:       getEnv("QDRANT_ADDR", "localhost:6333"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadAPIKeys() map[string]string {
	keys := make(map[string]string)
	val := os.Getenv("API_KEYS")
	if val == "" {
		return keys
	}
	for _, pair := range strings.Split(val, ",") {
		pair = strings.TrimSpace(pair)
		if parts := strings.SplitN(pair, ":", 2); len(parts) == 2 {
			keys[strings.TrimSpace(parts[1])] = strings.TrimSpace(parts[0])
		}
	}
	return keys
}

func initTelemetry(cfg *Config) func() {
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("observeid-identity-service"),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create telemetry resource")
	}

	// Initialize OTLP trace exporter using gRPC
	conn, err := grpc.DialContext(context.Background(), "localhost:4317",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Warn().Err(err).Msg("OTLP exporter not available, continuing without tracing")
		return func() {}
	}
	traceExporter, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithGRPCConn(conn),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create trace exporter")
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	exp, err := prometheus.New()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create metrics exporter")
	}
	mp := metric.NewMeterProvider(metric.WithReader(exp))
	otel.SetMeterProvider(mp)

	return func() {
		_ = tp.Shutdown(context.Background())
	}
}
