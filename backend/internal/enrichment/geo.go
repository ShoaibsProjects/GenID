package enrichment

import (
	"context"
	"log"
	"net/netip"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGZoneResolver resolves zones from the tenant_cidr_zones table
// (migration 006). The most specific CIDR match wins; no match defaults
// to "public" (the seeded 0.0.0.0/0 catch-all).
//
// MVP: local CIDR matching only — deterministic and offline. Real
// IP-to-city geolocation (ipapi.co / GeoLite2) is a Phase 5 swap behind
// this same interface; the workflow must not depend on external HTTP.
type PGZoneResolver struct {
	Pool *pgxpool.Pool
}

// Resolve implements ZoneResolver.
func (z *PGZoneResolver) Resolve(ctx context.Context, tenantID, ip string) (zone, location string) {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}
	if z.Pool == nil {
		return "public", locationForZone("public")
	}

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "public", locationForZone("public")
	}

	rows, err := z.Pool.Query(ctx, `
		SELECT zone_name, cidr::text
		FROM tenant_cidr_zones
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		log.Printf("[ENRICHMENT] zone query failed for tenant %s: %v", tenantID, err)
		return "public", locationForZone("public")
	}
	defer rows.Close()

	bestBits := -1
	bestZone := "public"
	for rows.Next() {
		var zoneName, cidrText string
		if err := rows.Scan(&zoneName, &cidrText); err != nil {
			continue
		}
		prefix, perr := netip.ParsePrefix(cidrText)
		if perr != nil || !prefix.Contains(addr) {
			continue
		}
		if prefix.Bits() > bestBits {
			bestBits = prefix.Bits()
			bestZone = zoneName
		}
	}

	return bestZone, locationForZone(bestZone)
}

// locationForZone maps a zone to a deterministic human-readable location.
// MVP mapping only — real geocoding replaces this in Phase 5.
func locationForZone(zone string) string {
	switch zone {
	case "corporate":
		return "us-office-sf"
	case "vpn":
		return "vpn-us"
	default:
		return "remote-unknown"
	}
}
