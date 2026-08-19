package contextual

import (
	"testing"
	"time"
)

var testSites = []Site{
	{ID: "us-office-sf", CIDRs: []string{"10.10.0.0/16"}, Lat: 37.7749, Lon: -122.4194},
	{ID: "us-office-nyc", CIDRs: []string{"10.20.0.0/16"}, Lat: 40.7128, Lon: -74.0060},
	{ID: "in-office-blr", CIDRs: []string{"10.30.0.0/16"}, Lat: 12.9716, Lon: 77.5946},
}

var testHours = BusinessHours{
	StartHour: 8, EndHour: 17,
	Weekdays: []int{1, 2, 3, 4, 5}, // Mon–Fri
	Timezone: "America/Los_Angeles",
}

func managedDevices(id string) string {
	if id == "jamf-1234" {
		return "managed"
	}
	return "unmanaged"
}

func newTestEnricher() *Enricher {
	return NewEnricher(testSites, testHours, managedDevices)
}

// Monday 2026-08-17, 10:00 PDT = business hours
var businessTime = time.Date(2026, 8, 17, 17, 0, 0, 0, time.UTC)

// Saturday 2026-08-15, 10:00 PDT = weekend = after hours
var weekendTime = time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC)

// Monday 23:00 PDT = weekday but outside window
var nightTime = time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)

func TestEnrichOfficeManagedBusinessHours(t *testing.T) {
	c := newTestEnricher().Enrich(businessTime, "10.10.1.50", "jamf-1234")
	if c.NetworkZone != "corporate" || c.Location != "us-office-sf" ||
		c.DeviceTrust != "managed" || c.TimeOfDay != "business_hours" {
		t.Fatalf("got %+v", c)
	}
	if !c.Trusted() {
		t.Fatal("office + managed + business hours must be trusted")
	}
}

func TestEnrichRemoteUnmanaged(t *testing.T) {
	c := newTestEnricher().Enrich(businessTime, "203.0.113.9", "personal-laptop")
	if c.NetworkZone != "public" || c.Location != "remote" || c.DeviceTrust != "unmanaged" {
		t.Fatalf("got %+v", c)
	}
	if c.Trusted() {
		t.Fatal("public + unmanaged must not be trusted")
	}
}

func TestEnrichUnknownIPFailsClosed(t *testing.T) {
	for _, ip := range []string{"", "not-an-ip", "999.1.1.1"} {
		c := newTestEnricher().Enrich(businessTime, ip, "jamf-1234")
		if c.NetworkZone != "unknown" {
			t.Fatalf("ip %q: expected unknown zone, got %s", ip, c.NetworkZone)
		}
		if c.Trusted() {
			t.Fatalf("ip %q: unknown zone must not be trusted", ip)
		}
	}
}

func TestEnrichStripsPort(t *testing.T) {
	c := newTestEnricher().Enrich(businessTime, "10.20.5.5:51234", "jamf-1234")
	if c.Location != "us-office-nyc" {
		t.Fatalf("RemoteAddr form not handled: %+v", c)
	}
}

func TestEnrichAfterHours(t *testing.T) {
	c := newTestEnricher().Enrich(weekendTime, "10.10.1.50", "jamf-1234")
	if c.TimeOfDay != "after_hours" || c.Trusted() {
		t.Fatalf("weekend should be after_hours/untrusted: %+v", c)
	}
	c = newTestEnricher().Enrich(nightTime, "10.10.1.50", "jamf-1234")
	if c.TimeOfDay != "after_hours" {
		t.Fatalf("23:00 weekday should be after_hours: %+v", c)
	}
}

func TestEnrichUnknownDeviceNeverManaged(t *testing.T) {
	e := NewEnricher(testSites, testHours, nil) // no MDM wired
	c := e.Enrich(businessTime, "10.10.1.50", "any-device")
	if c.DeviceTrust != "unknown" || c.Trusted() {
		t.Fatalf("nil checker must default to unknown/untrusted: %+v", c)
	}
}

func TestAdjustRiskBand(t *testing.T) {
	trusted := AccessContext{NetworkZone: "corporate", DeviceTrust: "managed", TimeOfDay: "business_hours"}
	untrusted := AccessContext{NetworkZone: "public", DeviceTrust: "unmanaged", TimeOfDay: "after_hours"}

	cases := []struct {
		band string
		ctx  AccessContext
		want string
	}{
		{"low", trusted, "minimal"},       // low-risk case: office + low risk → auto-approve
		{"elevated", trusted, "low"},
		{"minimal", trusted, "minimal"},   // floor
		{"low", untrusted, "elevated"},    // remote → step-up
		{"high", untrusted, "critical"},   // ceiling applied
		{"critical", untrusted, "critical"},
		{"", untrusted, "elevated"},       // unknown band starts at low → up one
	}
	for _, tc := range cases {
		if got := AdjustRiskBand(tc.band, tc.ctx); got != tc.want {
			t.Errorf("AdjustRiskBand(%q) = %q, want %q", tc.band, got, tc.want)
		}
	}
}

func TestImpossibleTravel(t *testing.T) {
	sf, nyc := testSites[0], testSites[1]

	// SF → NYC in 1 hour: impossible (4130 km needs ~4.6h at jet speed)
	if !ImpossibleTravel(sf, nyc, time.Hour) {
		t.Fatal("SF→NYC in 1h should be impossible travel")
	}
	// SF → NYC in 8 hours: possible
	if ImpossibleTravel(sf, nyc, 8*time.Hour) {
		t.Fatal("SF→NYC in 8h should be possible")
	}
	// Same site: never impossible
	if ImpossibleTravel(sf, sf, time.Minute) {
		t.Fatal("same site should never be impossible travel")
	}
	// Unknown site: fail-open
	if ImpossibleTravel(Site{}, nyc, time.Minute) {
		t.Fatal("unknown origin should fail open")
	}
}

func TestToCedarContextKeys(t *testing.T) {
	c := AccessContext{NetworkZone: "corporate", Location: "us-office-sf", DeviceTrust: "managed", TimeOfDay: "business_hours"}
	m := c.ToCedarContext()
	for _, k := range []string{"network_zone", "location", "device_trust", "time_of_day"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing Cedar context key %q", k)
		}
	}
}
