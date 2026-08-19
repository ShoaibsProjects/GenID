package enrichment

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPGResolversIntegration validates the real PG resolvers against the
// seeded tenant_cidr_zones + tenant_business_hours rows (migration 006).
// Skips when PostgreSQL is not reachable (e.g., plain `go test`).
func TestPGResolversIntegration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgresql://observeid:observeid@localhost:5432/observeid?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	defer pool.Close()

	tenant := "00000000-0000-0000-0000-000000000001"

	// 1) CIDR zones: 10.x → corporate, 172.16.x → vpn, 203.0.113.1 → public.
	zone, loc := (&PGZoneResolver{Pool: pool}).Resolve(ctx, tenant, "10.0.1.5")
	if zone != "corporate" || loc != "us-office-sf" {
		t.Errorf("10.0.1.5 → %s/%s, want corporate/us-office-sf", zone, loc)
	}
	zone, _ = (&PGZoneResolver{Pool: pool}).Resolve(ctx, tenant, "172.16.0.5")
	if zone != "vpn" {
		t.Errorf("172.16.0.5 → %s, want vpn", zone)
	}
	zone, _ = (&PGZoneResolver{Pool: pool}).Resolve(ctx, tenant, "203.0.113.1")
	if zone != "public" {
		t.Errorf("203.0.113.1 → %s, want public", zone)
	}

	// 2) Business hours: Monday 10:00 ET → business_hours, 22:00 → after_hours.
	et := time.FixedZone("EDT", -4*3600)
	hr := &PGBusinessHoursResolver{Pool: pool}
	monday10 := time.Date(2026, 8, 10, 10, 0, 0, 0, et)
	monday22 := time.Date(2026, 8, 10, 22, 0, 0, 0, et)
	if got := hr.Evaluate(ctx, tenant, monday10); got != "business_hours" {
		t.Errorf("Monday 10:00 ET → %q, want business_hours", got)
	}
	if got := hr.Evaluate(ctx, tenant, monday22); got != "after_hours" {
		t.Errorf("Monday 22:00 ET → %q, want after_hours", got)
	}

	// 3) Weekend classification (weekend_access = false for the seed).
	sunday := time.Date(2026, 8, 9, 10, 0, 0, 0, et)
	if got := hr.Evaluate(ctx, tenant, sunday); got != "weekend" {
		t.Errorf("Sunday 10:00 ET → %q, want weekend", got)
	}

	// 4) Full EnrichAt path against real tables (office/managed/low).
	svc := NewEnrichmentService(pool, nil)
	ec, err := svc.EnrichAt(ctx, tenant, ContextSignals{IPAddress: "10.0.1.5"}, "managed", 200, &monday10)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if ec.NetworkZone != "corporate" || ec.DeviceTrust != "managed" || ec.TimeOfDay != "business_hours" || ec.RiskBand != "low" {
		t.Errorf("enriched = %+v, want corporate/managed/business_hours/low", ec)
	}

	// 5) Longest-prefix specificity: an overlapping 10.0.0.0/16 corporate
	// subnet must win over 10.0.0.0/8 (insert, assert, clean up).
	cleanup := insertZone(t, pool, tenant, "corporate", "10.0.1.0/24")
	defer cleanup()
	zone, _ = (&PGZoneResolver{Pool: pool}).Resolve(ctx, tenant, "10.0.1.5")
	if zone != "corporate" {
		t.Errorf("10.0.1.5 with /24 override → %s, want corporate", zone)
	}
}

func insertZone(t *testing.T, pool *pgxpool.Pool, tenant, name, cidr string) func() {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenant_cidr_zones (tenant_id, zone_name, cidr, description) VALUES ($1, $2, $3::cidr, 'test-override')`,
		tenant, name, cidr)
	if err != nil {
		t.Fatalf("insert test zone: %v", err)
	}
	return func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM tenant_cidr_zones WHERE description = 'test-override' AND cidr = $1::cidr`, cidr)
	}
}

// sanity: net import used by the query types in case cidr scan needs it.
var _ = net.ParseIP
