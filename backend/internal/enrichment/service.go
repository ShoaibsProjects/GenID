package enrichment

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// ContextSignals are the raw, per-request signals collected at the edge
// (headers, client hints) before any enrichment happens.
type ContextSignals struct {
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	DeviceID  string `json:"device_id,omitempty"`
	MFAMethod string `json:"mfa_method,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// EnrichedContext is the evaluated, risk-annotated view of a request.
// All fields feed directly into Cedar conditional-access policies.
type EnrichedContext struct {
	Location    string `json:"location"`     // "us-office-sf", "remote-unknown"
	NetworkZone string `json:"network_zone"` // "corporate", "vpn", "public"
	DeviceTrust string `json:"device_trust"` // "managed", "unmanaged", "unknown"
	TimeOfDay   string `json:"time_of_day"`  // "business_hours", "after_hours", "weekend"
	GeoVelocity bool   `json:"geo_velocity"`
	RiskBand    string `json:"risk_band"`
	RiskScore   int    `json:"risk_score"`
}

// ZoneResolver maps an IP to its network zone + location.
type ZoneResolver interface {
	Resolve(ctx context.Context, tenantID, ip string) (zone, location string)
}

// HoursResolver classifies a time against the tenant's business hours.
type HoursResolver interface {
	Evaluate(ctx context.Context, tenantID string, at time.Time) string
}

// EnrichmentService orchestrates signal enrichment: zone resolution,
// device trust, business hours, and risk banding. Results are cached in
// Redis per (tenant, IP) for 5 minutes.
type EnrichmentService struct {
	zone  ZoneResolver
	hours HoursResolver
	cache *ContextCache
}

// NewEnrichmentService wires the orchestrator to PostgreSQL (zones,
// business hours) and Redis (context cache).
func NewEnrichmentService(pgPool *pgxpool.Pool, rdb *redis.Client) *EnrichmentService {
	return &EnrichmentService{
		zone:  &PGZoneResolver{Pool: pgPool},
		hours: &PGBusinessHoursResolver{Pool: pgPool},
		cache: NewContextCache(rdb, 5*time.Minute),
	}
}

// Enrich evaluates the signals at the current time. deviceTrust is the
// X-Device-Trust header value ("managed" | "unmanaged" | other).
func (s *EnrichmentService) Enrich(ctx context.Context, tenantID string, signals ContextSignals, deviceTrust string, riskScore int) (EnrichedContext, error) {
	return s.EnrichAt(ctx, tenantID, signals, deviceTrust, riskScore, nil)
}

// EnrichAt evaluates the signals at a fixed time (used by tests and the
// deterministic demo path); a nil at evaluates at time.Now(). Results are
// cached per (tenant, IP, trust, risk) with a 5-minute TTL; a cache hit
// skips geo/time resolution entirely. Fixed-clock (non-nil at) evaluations
// are kept on a separate cache partition so demo runs never reuse live data.
func (s *EnrichmentService) EnrichAt(ctx context.Context, tenantID string, signals ContextSignals, deviceTrust string, riskScore int, at *time.Time) (EnrichedContext, error) {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}
	if at == nil {
		now := time.Now()
		at = &now
	}

	if cached, err := s.cache.Get(ctx, tenantID, signals.IPAddress, deviceTrust, riskScore, at); err == nil && cached != nil {
		return *cached, nil
	}

	zone, location := s.zone.Resolve(ctx, tenantID, signals.IPAddress)
	trust := EvaluateDeviceTrust(deviceTrust)
	tod := s.hours.Evaluate(ctx, tenantID, *at)

	ec := EnrichedContext{
		Location:    location,
		NetworkZone: zone,
		DeviceTrust: trust,
		TimeOfDay:   tod,
		GeoVelocity: false, // MVP: single-signal evaluation; real velocity needs multi-signal tracking (Phase 5)
		RiskBand:    RiskBandFromScore(riskScore),
		RiskScore:   riskScore,
	}

	_ = s.cache.Set(ctx, tenantID, signals.IPAddress, deviceTrust, riskScore, at, &ec)
	return ec, nil
}

// RiskBandFromScore maps a risk score to a band. Thresholds are kept in
// sync with services.RiskBandFromScore (internal/services/access_service.go);
// enrichment cannot import services without an import cycle
// (services -> workflow -> activities -> enrichment).
func RiskBandFromScore(score int) string {
	switch {
	case score >= 800:
		return "critical"
	case score >= 600:
		return "high"
	case score >= 300:
		return "elevated"
	case score >= 100:
		return "low"
	default:
		return "minimal"
	}
}
