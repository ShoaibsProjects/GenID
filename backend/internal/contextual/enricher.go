// Package contextual implements the GenID Context Enrichment Service —
// the conditional-access brain that translates request signals (source IP,
// device, time) into the access context Cedar policies evaluate.
//
// v1 is deliberately dependency-free and deterministic: office locations are
// static CIDR maps, device trust is a lookup hook, business hours are config.
// Real MDM (Jamf/Intune/CrowdStrike) and GeoIP providers plug in behind the
// same interface later without touching workflows or policies.
package contextual

import (
	"net"
	"net/netip"
	"time"
)

// AccessContext is the enriched situational context attached to every
// access decision. Field names match the Cedar context keys exactly.
type AccessContext struct {
	NetworkZone string `json:"network_zone"` // "corporate" | "public" | "unknown"
	Location    string `json:"location"`     // site id, e.g. "us-office-sf", or "remote"
	DeviceTrust string `json:"device_trust"` // "managed" | "unmanaged" | "unknown"
	TimeOfDay   string `json:"time_of_day"`  // "business_hours" | "after_hours"
}

// Site is a known corporate location: an id and the CIDRs that belong to it.
type Site struct {
	ID     string   `json:"id"`     // e.g. "us-office-sf"
	CIDRs  []string `json:"cidrs"`  // e.g. ["10.10.0.0/16", "203.0.113.0/24"]
	Lat    float64  `json:"lat"`    // for impossible-travel checks
	Lon    float64  `json:"lon"`
}

// BusinessHours defines the corporate working window.
type BusinessHours struct {
	StartHour int    `json:"start_hour"` // inclusive, local time, e.g. 8
	EndHour   int    `json:"end_hour"`   // exclusive, e.g. 17 → 08:00–17:00
	Weekdays  []int  `json:"weekdays"`   // 0=Sunday … 6=Saturday; empty = every day
	Timezone  string `json:"timezone"`   // IANA name, e.g. "America/Los_Angeles"; empty = UTC
}

// DeviceChecker resolves a device id to its management state. Return
// "managed", "unmanaged", or "unknown". Wired to MDM inventory in v2.
type DeviceChecker func(deviceID string) string

// Enricher computes AccessContext from request signals.
type Enricher struct {
	sites      []parsedSite
	hours      BusinessHours
	loc        *time.Location
	checkDevice DeviceChecker
}

type parsedSite struct {
	Site
	prefixes []netip.Prefix
}

// NewEnricher builds an Enricher. Invalid CIDRs are skipped (logged by
// caller via the returned error count in Validate). A nil checker defaults
// to "unknown" device trust — safe default, never "managed".
func NewEnricher(sites []Site, hours BusinessHours, checker DeviceChecker) *Enricher {
	e := &Enricher{hours: hours, loc: time.UTC, checkDevice: checker}
	if e.checkDevice == nil {
		e.checkDevice = func(string) string { return "unknown" }
	}
	if hours.Timezone != "" {
		if loc, err := time.LoadLocation(hours.Timezone); err == nil {
			e.loc = loc
		}
	}
	for _, s := range sites {
		ps := parsedSite{Site: s}
		for _, c := range s.CIDRs {
			if p, err := netip.ParsePrefix(c); err == nil {
				ps.prefixes = append(ps.prefixes, p)
			}
		}
		e.sites = append(e.sites, ps)
	}
	return e
}

// Enrich computes the full access context for one request.
// sourceIP may be empty or malformed — that is a signal, not an error:
// it produces network_zone "unknown", which policies should treat as
// untrusted (fail-closed).
func (e *Enricher) Enrich(now time.Time, sourceIP, deviceID string) AccessContext {
	ctx := AccessContext{
		NetworkZone: "unknown",
		Location:    "unknown",
		DeviceTrust: e.checkDevice(deviceID),
		TimeOfDay:   e.timeOfDay(now),
	}
	if deviceID == "" {
		ctx.DeviceTrust = "unknown"
	}

	ip := parseIP(sourceIP)
	if ip == nil {
		return ctx
	}
	for _, s := range e.sites {
		for _, p := range s.prefixes {
			if p.Contains(*ip) {
				ctx.NetworkZone = "corporate"
				ctx.Location = s.ID
				return ctx
			}
		}
	}
	ctx.NetworkZone = "public"
	ctx.Location = "remote"
	return ctx
}

// ToCedarContext flattens the context into the map Cedar policies read.
func (c AccessContext) ToCedarContext() map[string]any {
	return map[string]any{
		"network_zone": c.NetworkZone,
		"location":     c.Location,
		"device_trust": c.DeviceTrust,
		"time_of_day":  c.TimeOfDay,
	}
}

// Trusted reports whether the context represents a fully trusted situation:
// corporate network, managed device, business hours. Policies use this for
// auto-approval decisions (auto-approval rule: office + low risk → auto-grant).
func (c AccessContext) Trusted() bool {
	return c.NetworkZone == "corporate" &&
		c.DeviceTrust == "managed" &&
		c.TimeOfDay == "business_hours"
}

// AdjustRiskBand translates context trust into a risk-band shift.
// Trusted context relaxes one band (low → minimal, enabling auto-approve);
// untrusted context tightens one band (remote/unmanaged → step-up).
// Bands: minimal, low, elevated, high, critical.
func AdjustRiskBand(band string, c AccessContext) string {
	order := []string{"minimal", "low", "elevated", "high", "critical"}
	idx := 1 // unknown bands start at "low"
	for i, b := range order {
		if b == band {
			idx = i
			break
		}
	}
	if c.Trusted() {
		idx--
	} else {
		idx++
	}
	if idx < 0 {
		idx = 0
	}
	if idx > len(order)-1 {
		idx = len(order) - 1
	}
	return order[idx]
}

// ImpossibleTravel reports whether moving from prevSite to newSite within
// elapsed time is physically impossible at commercial-jet speed
// (~900 km/h). Unknown sites never trigger — fail-open on missing data,
// because a false positive locks out legitimate travelers.
func ImpossibleTravel(prevSite, newSite Site, elapsed time.Duration) bool {
	if prevSite.ID == "" || newSite.ID == "" || prevSite.ID == newSite.ID {
		return false
	}
	distKm := haversineKm(prevSite.Lat, prevSite.Lon, newSite.Lat, newSite.Lon)
	needed := time.Duration(distKm/900.0*float64(time.Hour))
	return elapsed < needed
}

func (e *Enricher) timeOfDay(now time.Time) string {
	local := now.In(e.loc)
	h := e.hours
	if h.StartHour == 0 && h.EndHour == 0 {
		return "business_hours" // unconfigured → no restriction
	}
	if len(h.Weekdays) > 0 {
		wd := int(local.Weekday())
		ok := false
		for _, d := range h.Weekdays {
			if d == wd {
				ok = true
				break
			}
		}
		if !ok {
			return "after_hours"
		}
	}
	if local.Hour() >= h.StartHour && local.Hour() < h.EndHour {
		return "business_hours"
	}
	return "after_hours"
}

func parseIP(raw string) *netip.Addr {
	if raw == "" {
		return nil
	}
	// strip port if present (RemoteAddr form)
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return nil
	}
	return &addr
}
