package enrichment

import (
	"context"
	"testing"
	"time"
)

// fakeZoneResolver mirrors the seeded tenant_cidr_zones rows: 10.0.0.0/8 →
// corporate, 172.16.0.0/12 → vpn, everything else → public.
type fakeZoneResolver struct{}

func (f *fakeZoneResolver) Resolve(_ context.Context, _, ip string) (string, string) {
	switch {
	case ip == "10.0.1.5":
		return "corporate", locationForZone("corporate")
	case ip == "172.16.0.5":
		return "vpn", locationForZone("vpn")
	default:
		return "public", locationForZone("public")
	}
}

// fakeHoursResolver mirrors the seeded tenant_business_hours config:
// America/New_York 09:00–17:00 weekdays. Deterministic, no DB.
type fakeHoursResolver struct{}

func (f *fakeHoursResolver) Evaluate(_ context.Context, _ string, at time.Time) string {
	tz, _ := time.LoadLocation("America/New_York")
	local := at.In(tz)
	wd := int(local.Weekday())
	if wd == 0 || wd == 6 {
		return "weekend"
	}
	nowMins := local.Hour()*60 + local.Minute()
	if nowMins >= 9*60 && nowMins < 17*60 {
		return "business_hours"
	}
	return "after_hours"
}

func newTestService() *EnrichmentService {
	return &EnrichmentService{
		zone:  &fakeZoneResolver{},
		hours: &fakeHoursResolver{},
		cache: NewContextCache(nil, 0),
	}
}

// TestEnrichmentMatrix is the mandatory 6-case matrix (spec 4.5):
// zone, trust, and time classification per case, via the orchestrator.
func TestEnrichmentMatrix(t *testing.T) {
	ctx := context.Background()
	et := time.FixedZone("EDT", -4*3600)                // America/New_York, August (DST)
	monday10 := time.Date(2026, 8, 10, 10, 0, 0, 0, et) // Monday 10:00 ET
	monday22 := time.Date(2026, 8, 10, 22, 0, 0, 0, et) // Monday 22:00 ET

	cases := []struct {
		name      string
		ip        string
		device    string
		at        time.Time
		risk      int
		wantZone  string
		wantTrust string
		wantTime  string
		wantBand  string
	}{
		{name: "Office-Managed-Business-Low", ip: "10.0.1.5", device: "managed", at: monday10, risk: 200,
			wantZone: "corporate", wantTrust: "managed", wantTime: "business_hours", wantBand: "low"},
		{name: "Office-Unmanaged-Business-Low", ip: "10.0.1.5", device: "unmanaged", at: monday10, risk: 200,
			wantZone: "corporate", wantTrust: "unmanaged", wantTime: "business_hours", wantBand: "low"},
		{name: "Remote-Managed-Business-Low", ip: "203.0.113.1", device: "managed", at: monday10, risk: 200,
			wantZone: "public", wantTrust: "managed", wantTime: "business_hours", wantBand: "low"},
		{name: "Office-Managed-AfterHours-Low", ip: "10.0.1.5", device: "managed", at: monday22, risk: 200,
			wantZone: "corporate", wantTrust: "managed", wantTime: "after_hours", wantBand: "low"},
		{name: "Office-Managed-Business-Critical", ip: "10.0.1.5", device: "managed", at: monday10, risk: 850,
			wantZone: "corporate", wantTrust: "managed", wantTime: "business_hours", wantBand: "critical"},
		{name: "VPN-Managed-Business-Low", ip: "172.16.0.5", device: "managed", at: monday10, risk: 200,
			wantZone: "vpn", wantTrust: "managed", wantTime: "business_hours", wantBand: "low"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec, err := newTestService().EnrichAt(ctx, "", ContextSignals{IPAddress: tc.ip}, tc.device, tc.risk, &tc.at)
			if err != nil {
				t.Fatalf("enrich: %v", err)
			}
			if ec.NetworkZone != tc.wantZone {
				t.Errorf("zone = %q, want %q", ec.NetworkZone, tc.wantZone)
			}
			if ec.DeviceTrust != tc.wantTrust {
				t.Errorf("trust = %q, want %q", ec.DeviceTrust, tc.wantTrust)
			}
			if ec.TimeOfDay != tc.wantTime {
				t.Errorf("time_of_day = %q, want %q", ec.TimeOfDay, tc.wantTime)
			}
			if ec.RiskBand != tc.wantBand {
				t.Errorf("risk_band = %q, want %q", ec.RiskBand, tc.wantBand)
			}
			if ec.RiskScore != tc.risk {
				t.Errorf("risk_score = %d, want %d", ec.RiskScore, tc.risk)
			}
		})
	}
}

// TestEvaluateDeviceTrust covers the header-only MVP trust mapping.
func TestEvaluateDeviceTrust(t *testing.T) {
	if got := EvaluateDeviceTrust("managed"); got != "managed" {
		t.Errorf("managed → %q", got)
	}
	if got := EvaluateDeviceTrust("unmanaged"); got != "unmanaged" {
		t.Errorf("unmanaged → %q", got)
	}
	for _, h := range []string{"", "jailbroken", "corporate-issued"} {
		if got := EvaluateDeviceTrust(h); got != "unknown" {
			t.Errorf("%q → %q, want unknown", h, got)
		}
	}
}

// TestRiskBandBoundaries pins the band thresholds (kept in sync with
// services.RiskBandFromScore).
func TestRiskBandBoundaries(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{0, "minimal"}, {99, "minimal"}, {100, "low"}, {299, "low"},
		{300, "elevated"}, {599, "elevated"}, {600, "high"}, {799, "high"},
		{800, "critical"}, {850, "critical"}, {1000, "critical"},
	}
	for _, tc := range cases {
		if got := RiskBandFromScore(tc.score); got != tc.want {
			t.Errorf("RiskBandFromScore(%d) = %q, want %q", tc.score, got, tc.want)
		}
	}
}
