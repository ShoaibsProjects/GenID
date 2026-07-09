package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	"github.com/observeid/identity-platform/internal/service"
	"github.com/observeid/identity-platform/internal/workflow"
	"github.com/observeid/identity-platform/pkg/telemetry"
)

func main() {
	// ─── Initialize Structured Logger ─────────────────────
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Str("service", "observeid-identity").
		Logger()

	log.Info().Msg("═══════════════════════════════════════════")
	log.Info().Msg("  ObserveID Identity Service Starting")
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
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: "",
		DB:       0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer rdb.Close()
	log.Info().Msg("Redis connected")

	// ─── Initialize Temporal Client ───────────────────────
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalHost,
		Namespace: "critical-offboarding",
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Temporal")
	}
	defer temporalClient.Close()
	log.Info().Msg("Temporal connected")

	// ─── Initialize Services ──────────────────────────────
	svc := service.NewIdentityService(pgPool, neo4jDriver, rdb, temporalClient)
	auditLogStore := svc.AuditStore()

	// ─── Start Temporal Worker ────────────────────────────
	w := worker.New(temporalClient, "critical-offboarding", worker.Options{
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

	act := activities.NewActivityService(pgPool, neo4jDriver, rdb, temporalClient)
	w.RegisterActivity(act)

	if err := w.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start Temporal worker")
	}
	defer w.Stop()
	log.Info().Msg("Temporal worker started")

	// ─── Start HTTP/gRPC Server ───────────────────────────
	r := mux.NewRouter()
	r.Use(corsMiddleware)
	r.Use(otelhttp.NewMiddleware("observeid-api"))
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
		// Root — API landing page
		r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{
  "service": "ObserveID Identity Fabric Engine",
  "version": "1.0.0",
  "status": "running",
  "docs": {
    "health":   "/health",
    "ready":    "/ready",
    "metrics":  "/metrics",
    "scim":     "/scim/v2/Users",
    "identities": "/api/v1/identities",
    "agents":    "/api/v1/agents",
    "access":    "/api/v1/access/check",
    "copilot":   "/api/v1/copilot/query",
    "caep":      "/api/v1/caep/events",
    "connectors": "/api/v1/connectors",
    "lcm":       "/api/v1/lcm",
    "groups":    "/api/v1/groups"
  }
}`)
		}).Methods("GET")
	}

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","service":"observeid-identity","version":"1.0.0"}`)
	}).Methods("GET")

	r.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check dependencies
		if err := rdb.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"unavailable","reason":"redis_down"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ready"}`)
	}).Methods("GET")

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
	api.HandleFunc("/access/check", svc.CheckAccess).Methods("POST")
	api.HandleFunc("/access/grant", svc.GrantAccess).Methods("POST")
	api.HandleFunc("/access/revoke", svc.RevokeAccess).Methods("POST")

	// AI Copilot API
	api.HandleFunc("/copilot/query", svc.CopilotQuery).Methods("POST")

	// CAEP API
	api.HandleFunc("/caep/events", svc.ListCAEPEvents).Methods("GET")
	api.HandleFunc("/caep/broadcast", svc.BroadcastCAEP).Methods("POST")

	// ─── Connector Management ───────────────────────
	api.HandleFunc("/connectors", svc.ListConnectors).Methods("GET")
	api.HandleFunc("/connectors", svc.CreateConnector).Methods("POST")
	api.HandleFunc("/connectors/test", svc.TestConnectorConnection).Methods("POST")
	api.HandleFunc("/connectors/{id}", svc.GetConnector).Methods("GET")
	api.HandleFunc("/connectors/{id}", svc.DeleteConnector).Methods("DELETE")
	api.HandleFunc("/connectors/{id}/connect", svc.ConnectConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/disconnect", svc.DisconnectConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/sync", svc.SyncConnector).Methods("POST")
	api.HandleFunc("/connectors/{id}/users", svc.GetConnectorUsers).Methods("GET")
	api.HandleFunc("/connectors/{id}/identities", svc.GetConnectorIdentities).Methods("GET")

	// ─── IAM Lifecycle Management (LCM) ────────────
	api.HandleFunc("/lcm", svc.ExecuteLCM).Methods("POST")
	api.HandleFunc("/lcm/history", svc.GetLCMHistory).Methods("GET")

	// ─── Identity CRUD ─────────────────────────────
	api.HandleFunc("/identities", svc.CreateIdentityRecord).Methods("POST")
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
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("Server failed")
	}
	log.Info().Msg("Server stopped")
}

// ─── CORS Middleware ───────────────────────────────────────
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type Config struct {
	DatabaseURL  string
	Neo4jURI     string
	Neo4jUser    string
	Neo4jPassword string
	RedisAddr    string
	TemporalHost string
	QdrantAddr   string
}

func loadConfig() *Config {
	return &Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgresql://observeid:observeid@localhost:5432/observeid?sslmode=disable"),
		Neo4jURI:      getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:     getEnv("NEO4J_USER", "neo4j"),
		Neo4jPassword: getEnv("NEO4J_PASSWORD", "observeid123"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		TemporalHost:  getEnv("TEMPORAL_HOST", "localhost:7233"),
		QdrantAddr:    getEnv("QDRANT_ADDR", "localhost:6333"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
