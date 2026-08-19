package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/observeid/genid/internal/mcp"
)

func main() {
	cfg := loadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := mcp.NewDeps(
		ctx,
		cfg.DatabaseURL,
		cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword,
		cfg.TemporalHost, cfg.TemporalNamespace,
		cfg.MCPAPIKey, "mcp-stdio",
	)
	if err != nil {
		log.Fatalf("[mcp] dependency init failed: %v", err)
	}
	defer deps.Close(ctx)

	switch cfg.Transport {
	case "sse":
		if err := mcp.ServeSSE(ctx, deps, cfg.Addr); err != nil {
			log.Fatalf("[mcp] sse server: %v", err)
		}
	default:
		if err := mcp.ServeStdio(deps); err != nil {
			log.Fatalf("[mcp] stdio server: %v", err)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() struct {
	DatabaseURL       string
	Neo4jURI          string
	Neo4jUser         string
	Neo4jPassword     string
	TemporalHost      string
	TemporalNamespace string
	MCPAPIKey         string
	Transport         string
	Addr              string
} {
	return struct {
		DatabaseURL       string
		Neo4jURI          string
		Neo4jUser         string
		Neo4jPassword     string
		TemporalHost      string
		TemporalNamespace string
		MCPAPIKey         string
		Transport         string
		Addr              string
	}{
		DatabaseURL:       getEnv("DATABASE_URL", "postgresql://observeid:observeid@localhost:5432/observeid?sslmode=disable"),
		Neo4jURI:          getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Neo4jUser:         getEnv("NEO4J_USER", "neo4j"),
		Neo4jPassword:     getEnv("NEO4J_PASSWORD", "observeid123"),
		TemporalHost:      getEnv("TEMPORAL_HOST", "localhost:7233"),
		TemporalNamespace: getEnv("TEMPORAL_NAMESPACE", "critical-offboarding"),
		MCPAPIKey:         os.Getenv("MCP_API_KEY"),
		Transport:         getEnv("MCP_TRANSPORT", "stdio"),
		Addr:              getEnv("MCP_ADDR", ":8099"),
	}
}
