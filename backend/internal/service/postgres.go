package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── PostgreSQL Connection Pool ───────────────────────────

func NewPostgresPool(databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Adaptive pool sizing: cloud managed Postgres has lower connection limits
	isCloud := strings.Contains(databaseURL, "neon.tech") ||
		strings.Contains(databaseURL, "railway.app") ||
		strings.Contains(databaseURL, "supabase") ||
		strings.Contains(databaseURL, "render.com") ||
		strings.Contains(databaseURL, "fly.dev")

	if isCloud {
		config.MaxConns = 5
		config.MinConns = 1
		config.MaxConnLifetime = 10 * time.Minute
		config.MaxConnIdleTime = 2 * time.Minute
	} else {
		config.MaxConns = 50
		config.MinConns = 5
		config.MaxConnLifetime = 30 * time.Minute
		config.MaxConnIdleTime = 5 * time.Minute
	}
	config.HealthCheckPeriod = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}
