package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ContextCache caches the full EnrichedContext JSON per (tenant, IP) with
// a configurable TTL (5 minutes for the MVP).
type ContextCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewContextCache wraps Redis; a nil client makes Get/Set no-ops so the
// enrichment service degrades gracefully when Redis is unavailable.
func NewContextCache(rdb *redis.Client, ttl time.Duration) *ContextCache {
	return &ContextCache{rdb: rdb, ttl: ttl}
}

// cacheKey fingerprints the request: two requests may share an IP but
// differ in device trust, risk score, or evaluation clock, and a fixed
// evaluate_at (demo/tests) must never reuse other clock's evaluations.
func cacheKey(tenantID, ip, deviceTrust string, riskScore int, evaluateAt *time.Time) string {
	clock := "live"
	if evaluateAt != nil {
		clock = strconv.FormatInt(evaluateAt.Unix(), 10)
	}
	return fmt.Sprintf("ctx:%s:%s:%s:%d:%s", tenantID, ip, deviceTrust, riskScore, clock)
}

// Get returns the cached context, or nil on miss / Redis failure.
func (c *ContextCache) Get(ctx context.Context, tenantID, ip, deviceTrust string, riskScore int, evaluateAt *time.Time) (*EnrichedContext, error) {
	if c.rdb == nil || tenantID == "" || ip == "" {
		return nil, nil
	}
	raw, err := c.rdb.Get(ctx, cacheKey(tenantID, ip, deviceTrust, riskScore, evaluateAt)).Bytes()
	if err != nil {
		return nil, nil // miss or Redis down → treat as miss
	}
	var ec EnrichedContext
	if err := json.Unmarshal(raw, &ec); err != nil {
		return nil, err
	}
	return &ec, nil
}

// Set writes the context with the configured TTL. Errors are non-fatal for
// the caller (cache is an optimization, not a guarantee).
func (c *ContextCache) Set(ctx context.Context, tenantID, ip, deviceTrust string, riskScore int, evaluateAt *time.Time, ec *EnrichedContext) error {
	if c.rdb == nil || tenantID == "" || ip == "" || ec == nil {
		return nil
	}
	raw, err := json.Marshal(ec)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, cacheKey(tenantID, ip, deviceTrust, riskScore, evaluateAt), raw, c.ttl).Err()
}
