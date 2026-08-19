package enrichment

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Default business hours used when the tenant has no tenant_business_hours
// row (or the table is empty): 09:00–17:00 America/New_York, weekdays only.
const (
	defaultTZ          = "America/New_York"
	defaultStartHour   = 9
	defaultStartMinute = 0
	defaultEndHour     = 17
	defaultEndMinute   = 0
)

// weekdayColumn maps Go's time.Weekday (Sunday=0) to the SQL column names
// of tenant_business_hours.
var weekdayColumn = [7]string{
	"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday",
}

// PGBusinessHoursResolver classifies `at` against the tenant's
// tenant_business_hours row (migration 006): "business_hours",
// "after_hours", or "weekend". Missing rows fall back to 09:00–17:00
// America/New_York weekdays.
type PGBusinessHoursResolver struct {
	Pool *pgxpool.Pool
}

// Evaluate implements HoursResolver.
func (h *PGBusinessHoursResolver) Evaluate(ctx context.Context, tenantID string, at time.Time) string {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	loc := defaultTZ
	starts := [7][2]int{} // [weekday][startHour, startMinute] — zero value = unset
	ends := [7][2]int{}
	weekendAccess := false

	if h.Pool != nil {
		cols := ""
		for i, name := range weekdayColumn {
			if i > 0 {
				cols += ", "
			}
			cols += name + "_start, " + name + "_end"
		}
		query := "SELECT timezone, " + cols + ", weekend_access FROM tenant_business_hours WHERE tenant_id = $1 LIMIT 1"
		rows, err := h.Pool.Query(ctx, query, tenantID)
		if err != nil {
			log.Printf("[ENRICHMENT] business hours query failed for tenant %s: %v", tenantID, err)
		} else {
			var vals [14]*time.Time
			scanArgs := []any{&loc}
			for i := 0; i < 14; i++ {
				scanArgs = append(scanArgs, &vals[i])
			}
			scanArgs = append(scanArgs, &weekendAccess)
			if rows.Next() {
				if err := rows.Scan(scanArgs...); err != nil {
					log.Printf("[ENRICHMENT] business hours scan failed for tenant %s: %v", tenantID, err)
				} else {
					for i := 0; i < 7; i++ {
						if vals[i*2] != nil {
							starts[i][0] = vals[i*2].Hour()
							starts[i][1] = vals[i*2].Minute()
						}
						if vals[i*2+1] != nil {
							ends[i][0] = vals[i*2+1].Hour()
							ends[i][1] = vals[i*2+1].Minute()
						}
					}
				}
			}
			rows.Close()
		}
	}

	tz, err := time.LoadLocation(loc)
	if err != nil {
		tz = time.UTC
	}
	local := at.In(tz)
	wd := int(local.Weekday())

	// Weekend with weekend_access disabled → "weekend" regardless of hours.
	if wd == 0 || wd == 6 {
		if !weekendAccess {
			return "weekend"
		}
	}

	// No configured row for this day → fall back to the 09:00–17:00 default.
	startH, startM := starts[wd][0], starts[wd][1]
	endH, endM := ends[wd][0], ends[wd][1]
	configured := starts[wd][0] != 0 || starts[wd][1] != 0 || ends[wd][0] != 0 || ends[wd][1] != 0
	if !configured {
		startH, startM = defaultStartHour, defaultStartMinute
		endH, endM = defaultEndHour, defaultEndMinute
	}

	nowMins := local.Hour()*60 + local.Minute()
	startMins := startH*60 + startM
	endMins := endH*60 + endM
	if nowMins >= startMins && nowMins < endMins {
		return "business_hours"
	}
	return "after_hours"
}
