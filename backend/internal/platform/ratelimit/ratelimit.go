// Package ratelimit enforces request rate limits per API key and per tenant (ARCHITECTURE.md §9,
// "noisy neighbors") with GCRA (the generic cell rate algorithm) in Redis.
//
// One Lua script checks both limits atomically and consumes from both only when both allow, so a
// rejected request never spends budget. Time comes from the Redis server clock, so every API
// instance sees the same limits.
package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Limit is a sustained rate with a burst allowance.
type Limit struct {
	PerSecond float64
	Burst     int
}

// Override replaces the platform's per-key and per-tenant limits for one tenant (set by RiskOS
// staff, ADR-0010). It never changes who is limited, only how much.
type Override struct {
	PerKey    Limit
	PerTenant Limit
}

// Overrides finds a tenant's override. A lookup that fails reports none, so limiting falls back
// to the platform's limits instead of failing requests.
type Overrides interface {
	Override(ctx context.Context, tenantID uuid.UUID) (Override, bool)
}

// Decision is the outcome of a check.
type Decision struct {
	Allowed bool
	// RetryAfter is how long until the request would be allowed (when not allowed).
	RetryAfter time.Duration
}

// Limiter checks limits in Redis.
type Limiter struct {
	rdb     *redis.Client
	script  *redis.Script
	peek    *redis.Script
	timeout time.Duration
}

// New returns a Limiter on rdb. Each check is bounded by timeout (0: the caller's context
// only), so a slow or unreachable Redis costs a request at most that long.
func New(rdb *redis.Client, timeout time.Duration) *Limiter {
	return &Limiter{rdb: rdb, script: redis.NewScript(gcraScript), peek: redis.NewScript(peekScript), timeout: timeout}
}

// gcraScript checks N keys, each with its own emission interval and burst tolerance, and
// advances all of them only if every one allows the request.
//
// KEYS[i]: limit keys. ARGV[2i-1]: emission interval (µs). ARGV[2i]: burst tolerance (µs).
// Returns {1, 0} when allowed, {0, retry_after_µs} otherwise.
const gcraScript = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
local new_tats = {}
local wait = 0
for i, key in ipairs(KEYS) do
  local interval = tonumber(ARGV[2 * i - 1])
  local tolerance = tonumber(ARGV[2 * i])
  local tat = tonumber(redis.call('GET', key) or now)
  if tat < now then tat = now end
  local new_tat = tat + interval
  local allow_at = new_tat - tolerance - interval
  if allow_at > now then
    local w = allow_at - now
    if w > wait then wait = w end
  end
  new_tats[i] = new_tat
end
if wait > 0 then
  return {0, wait}
end
for i, key in ipairs(KEYS) do
  redis.call('SET', key, new_tats[i], 'PX', math.ceil((new_tats[i] - now) / 1000) + 1000)
end
return {1, 0}
`

// peekScript is gcraScript for one key without consuming: would a request be allowed now?
const peekScript = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
local interval = tonumber(ARGV[1])
local tolerance = tonumber(ARGV[2])
local tat = tonumber(redis.call('GET', KEYS[1]) or now)
if tat < now then tat = now end
local allow_at = tat + interval - tolerance - interval
if allow_at > now then
  return {0, allow_at - now}
end
return {1, 0}
`

// Rule binds a Redis key to a limit.
type Rule struct {
	Key   string
	Limit Limit
}

// Peek reports whether a request would be allowed under rule now, without consuming it.
func (l *Limiter) Peek(ctx context.Context, rule Rule) (Decision, error) {
	if l.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.timeout)
		defer cancel()
	}
	interval := 1e6 / rule.Limit.PerSecond
	res, err := l.peek.Run(ctx, l.rdb, []string{rule.Key},
		int64(math.Ceil(interval)), int64(math.Ceil(interval*float64(rule.Limit.Burst-1)))).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("rate limit peek: %w", err)
	}
	if len(res) != 2 {
		return Decision{}, fmt.Errorf("rate limit peek: unexpected reply %v", res)
	}
	if res[0] == 1 {
		return Decision{Allowed: true}, nil
	}
	return Decision{RetryAfter: time.Duration(res[1]) * time.Microsecond}, nil
}

// Allow checks every rule and consumes one request from each only if all allow it.
func (l *Limiter) Allow(ctx context.Context, rules ...Rule) (Decision, error) {
	keys := make([]string, len(rules))
	args := make([]any, 0, 2*len(rules))
	for i, r := range rules {
		keys[i] = r.Key
		interval := 1e6 / r.Limit.PerSecond
		args = append(args, int64(math.Ceil(interval)), int64(math.Ceil(interval*float64(r.Limit.Burst-1))))
	}
	if l.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.timeout)
		defer cancel()
	}
	res, err := l.script.Run(ctx, l.rdb, keys, args...).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("rate limit check: %w", err)
	}
	if len(res) != 2 {
		return Decision{}, fmt.Errorf("rate limit check: unexpected reply %v", res)
	}
	if res[0] == 1 {
		return Decision{Allowed: true}, nil
	}
	return Decision{RetryAfter: time.Duration(res[1]) * time.Microsecond}, nil
}
