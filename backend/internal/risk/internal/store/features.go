package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
)

// Retention of feature state in Redis.
const (
	historyTTL      = 180 * 24 * time.Hour // seen sets, amount baselines, event counters
	credentialTTL   = 7 * 24 * time.Hour
	fanInDayTTL     = 49 * time.Hour
	amountBaselineN = 500 // the baseline behaves like a window of about this many transactions
)

// Redis is the client the feature store uses; *redis.Client and *redis.ClusterClient satisfy it.
type Redis = redis.UniversalClient

// FeatureStore reads and updates behavioral features in Redis.
//
// Identifiers never reach Redis in the clear: every entity key uses an HMAC of (tenant,
// environment, kind, identifier) under a server-side key (ARCHITECTURE.md §16.3). Each entity's
// keys share a Redis Cluster hash tag, so multi-key operations stay on one shard.
type FeatureStore struct {
	rdb      Redis
	hashKey  []byte
	timeout  time.Duration
	seen     *redis.Script
	velocity *redis.Script
	amount   *redis.Script
	window   *redis.Script
	profile  *redis.Script
	export   *redis.Script
	// scriptsLoaded is set once the pipelined scripts are known to be in the Redis script
	// cache, so Observe can send EVALSHA instead of the whole script body (which costs Redis a
	// hash and a lookup on every call). Cleared when Redis reports NOSCRIPT (after a restart).
	scriptsLoaded atomic.Bool
	// state reports a warm-up after a reset; nil when reset detection is off.
	state *StateWatcher
	// baselineMaxPerHour caps the amount observations one user's baseline takes per hour in one
	// currency; zero means no cap.
	baselineMaxPerHour int
}

// WithBaselineCap limits how many transactions per hour teach one user's amount baseline in one
// currency (RISK_ENGINE.md, behavioral baselines). Zero removes the limit.
func (s *FeatureStore) WithBaselineCap(perHour int) *FeatureStore {
	s.baselineMaxPerHour = perHour
	return s
}

// WithState makes observations report a history warm-up while w detects one.
func (s *FeatureStore) WithState(w *StateWatcher) *FeatureStore {
	s.state = w
	return s
}

// NewFeatureStore returns a store. timeout bounds each observation (one Redis round trip).
func NewFeatureStore(rdb Redis, hashKey []byte, timeout time.Duration) *FeatureStore {
	return &FeatureStore{
		rdb: rdb, hashKey: hashKey, timeout: timeout,
		seen:     redis.NewScript(seenScript),
		velocity: redis.NewScript(velocityScript),
		amount:   redis.NewScript(amountScript),
		window:   redis.NewScript(windowScript),
		profile:  redis.NewScript(profileScript),
		export:   redis.NewScript(exportScript),
	}
}

// seenScript: in write mode add ARGV[1] to the set KEYS[1] (bounded to ARGV[3] members) and
// return 1 if it was new; in read mode only report whether it is new.
const seenScript = `
if ARGV[2] == '1' then
  local added = redis.call('SADD', KEYS[1], ARGV[1])
  if added == 1 and redis.call('SCARD', KEYS[1]) > tonumber(ARGV[3]) then
    redis.call('SPOP', KEYS[1])
  end
  redis.call('EXPIRE', KEYS[1], ARGV[4])
  return added
end
return 1 - redis.call('SISMEMBER', KEYS[1], ARGV[1])
`

// velocityScript keeps per-minute and per-hour counters for the entity KEYS[1] (derived keys
// share its hash tag) on the Redis clock. In write mode (ARGV[1] == '1') it counts the current
// event first. Returns counts for the last 1, 15 and 60 minutes and the last 24 hours.
const velocityScript = `
local t = redis.call('TIME')
local now = tonumber(t[1])
local minute = math.floor(now / 60)
local hour = math.floor(now / 3600)
if ARGV[1] == '1' then
  local mk = KEYS[1] .. ':m:' .. minute
  redis.call('INCR', mk)
  redis.call('EXPIRE', mk, 3900)
  local hk = KEYS[1] .. ':h:' .. hour
  redis.call('INCR', hk)
  redis.call('EXPIRE', hk, 90000)
end
local mkeys = {}
for i = 0, 59 do mkeys[#mkeys + 1] = KEYS[1] .. ':m:' .. (minute - i) end
local mv = redis.call('MGET', unpack(mkeys))
local m1, m15, m60 = 0, 0, 0
for i, v in ipairs(mv) do
  local n = tonumber(v) or 0
  if i == 1 then m1 = n end
  if i <= 15 then m15 = m15 + n end
  m60 = m60 + n
end
local hkeys = {}
for i = 0, 23 do hkeys[#hkeys + 1] = KEYS[1] .. ':h:' .. (hour - i) end
local hv = redis.call('MGET', unpack(hkeys))
local day = 0
for _, v in ipairs(hv) do day = day + (tonumber(v) or 0) end
return {m1, m15, m60, day}
`

// amountScript adds ARGV[1] to a running mean/variance (Welford). Beyond ARGV[2] observations
// the history is scaled down so the baseline tracks recent behavior and a single burst of
// unusual amounts cannot dominate it.
//
// ARGV[4] caps the updates one baseline takes per clock hour (ARGV[5], the hour number); 0 means
// no cap. Beyond it the amount is not learned and the script returns -1, so a burst of
// transactions, or an attacker spreading small amounts to drag the mean, moves the baseline
// by at most that many observations an hour.
const amountScript = `
local maxu = tonumber(ARGV[4])
if maxu > 0 then
  local hour = tonumber(ARGV[5])
  local uh = tonumber(redis.call('HGET', KEYS[1], 'uh') or '-1')
  local uc = tonumber(redis.call('HGET', KEYS[1], 'uc') or '0')
  if uh ~= hour then
    uh = hour
    uc = 0
  end
  if uc >= maxu then
    return -1
  end
  redis.call('HSET', KEYS[1], 'uh', uh, 'uc', uc + 1)
end
local n = tonumber(redis.call('HGET', KEYS[1], 'n') or '0')
local mean = tonumber(redis.call('HGET', KEYS[1], 'mean') or '0')
local m2 = tonumber(redis.call('HGET', KEYS[1], 'm2') or '0')
local x = tonumber(ARGV[1])
local cap = tonumber(ARGV[2])
if n >= cap then
  m2 = m2 * (cap - 1) / n
  n = cap - 1
end
n = n + 1
local delta = x - mean
mean = mean + delta / n
m2 = m2 + delta * (x - mean)
redis.call('HSET', KEYS[1], 'n', n, 'mean', string.format('%.6f', mean), 'm2', string.format('%.6f', m2))
redis.call('EXPIRE', KEYS[1], ARGV[3])
return n
`

type keys struct {
	scope tenancy.Scope
	hash  func(kind, value string) string
}

func (s *FeatureStore) keys(scope tenancy.Scope) keys {
	return keys{scope: scope, hash: func(kind, value string) string {
		m := hmac.New(sha256.New, s.hashKey)
		for _, part := range []string{scope.TenantID.String(), scope.EnvironmentID.String(), kind, value} {
			m.Write([]byte(part))
			m.Write([]byte{0})
		}
		return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(m.Sum(nil)[:16])
	}}
}

// entity returns the key of an entity: "rk:<tenant>:<env>:{<kind>:<hmac>}". The braces are the
// Redis Cluster hash tag.
func (k keys) entity(kind, value string) string {
	return fmt.Sprintf("rk:%s:%s:{%s:%s}", k.scope.TenantID, k.scope.EnvironmentID, kind, k.hash(kind, value))
}

// Observe reads (and in write mode updates) every feature the event needs, in one pipelined
// round trip bounded by the store timeout. Failures degrade the affected sources instead of
// failing the evaluation. Counts include the current event in both modes.
func (s *FeatureStore) Observe(ctx context.Context, e features.Event, write bool) features.Values {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return features.Values{Degraded: allSources()}
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	k := s.keys(scope)
	user := k.entity("u", e.Actor.UserID)
	w := boolArg(write)
	s.loadScripts(ctx)
	pipe := s.rdb.Pipeline()
	var scripted []*redis.Cmd
	run := func(script *redis.Script, keys []string, args ...any) *redis.Cmd {
		cmd := script.EvalSha(ctx, pipe, keys, args...)
		scripted = append(scripted, cmd)
		return cmd
	}

	// history
	// Events seen for the user: write mode counts this one (1 means first ever); read mode checks
	// whether any exist.
	var userCount *redis.IntCmd
	if write {
		userCount = pipe.Incr(ctx, user+":n")
		pipe.Expire(ctx, user+":n", historyTTL)
	} else {
		userCount = pipe.Exists(ctx, user+":n")
	}
	seen := func(set, member string) *redis.Cmd {
		return run(s.seen, []string{set}, member, w, domain.SeenSetCap, int(historyTTL.Seconds()))
	}
	var newDevice, newBeneficiary, newCountry *redis.Cmd
	if e.Actor.DeviceID != "" {
		newDevice = seen(user+":dev", k.hash("dev", e.Actor.DeviceID))
	}
	if b := e.BeneficiaryID(); b != "" {
		newBeneficiary = seen(user+":ben", k.hash("ben", b))
	}
	if e.Context.Country != "" {
		newCountry = seen(user+":cty", e.Context.Country)
	}
	// Money flows between the user and its counterparties (PON-01): the senders of incoming
	// payments are remembered next to the payees, with the same hash, so a counterparty that both
	// sent and received is recognized.
	var senderRelated, toPriorSender *redis.BoolCmd
	var newSender *redis.Cmd
	if t := e.Transaction; t != nil && t.Direction == "credit" && e.Compliance != nil && e.Compliance.Refs["sender"] != "" {
		sender := k.hash("ben", e.Compliance.Refs["sender"])
		senderRelated = pipe.SIsMember(ctx, user+":ben", sender)
		// seen adds the sender in write mode and only asks in a dry run.
		newSender = seen(user+":snd", sender)
	}
	if e.IsDebit() && e.BeneficiaryID() != "" {
		toPriorSender = pipe.SIsMember(ctx, user+":snd", k.hash("ben", e.BeneficiaryID()))
	}
	var credChange *redis.StringCmd
	if write && e.CredentialChange() {
		pipe.Set(ctx, user+":cred", time.Now().Unix(), credentialTTL)
	}
	if e.IsDebit() {
		credChange = pipe.Get(ctx, user+":cred")
	}
	var amount *redis.SliceCmd
	if e.IsDebit() {
		amount = pipe.HMGet(ctx, user+":amt:"+e.Transaction.Currency, "n", "mean", "m2")
	}

	// velocity
	var accountVel, loginVel *redis.Cmd
	if e.Transaction != nil {
		account := e.Actor.AccountID
		if account == "" {
			account = e.Actor.UserID
		}
		accountVel = run(s.velocity, []string{k.entity("a", account) + ":txn"}, w)
	}
	if e.Login != nil {
		failed := boolArg(write && e.Login.Outcome == "failure")
		loginVel = run(s.velocity, []string{user + ":fail"}, failed)
	}

	// network
	var deviceUsers, fanIn *redis.IntCmd
	if e.Actor.DeviceID != "" {
		devUsers := k.entity("d", e.Actor.DeviceID) + ":users"
		if write {
			pipe.PFAdd(ctx, devUsers, k.hash("u", e.Actor.UserID))
			pipe.Expire(ctx, devUsers, historyTTL)
		}
		deviceUsers = pipe.PFCount(ctx, devUsers)
	}
	if e.Transaction != nil && e.BeneficiaryID() != "" {
		// Distinct senders today and yesterday approximate a 24-hour window with daily keys.
		ben := k.entity("b", e.BeneficiaryID())
		now := time.Now().UTC()
		today := ben + ":s:" + now.Format("20060102")
		yesterday := ben + ":s:" + now.Add(-24*time.Hour).Format("20060102")
		if write {
			pipe.PFAdd(ctx, today, k.hash("u", e.Actor.UserID))
			pipe.Expire(ctx, today, fanInDayTTL)
		}
		fanIn = pipe.PFCount(ctx, today, yesterday)
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) && !isCommandError(err) {
		// Timeout or connection failure: nothing can be trusted.
		return features.Values{Degraded: allSources()}
	}
	for _, cmd := range scripted {
		if redis.HasErrorPrefix(cmd.Err(), "NOSCRIPT") {
			// Redis lost its script cache; this event's script features degrade (as any failed
			// command does) and the next observation reloads the scripts.
			s.scriptsLoaded.Store(false)
			break
		}
	}

	v := features.Values{Warming: s.state.Warming(time.Now())}
	degrade := func(source string) {
		for _, d := range v.Degraded {
			if d == source {
				return
			}
		}
		v.Degraded = append(v.Degraded, source)
	}
	current := int64(1)
	if write {
		current = 0 // write mode already counted the event
	}

	if n, err := userCount.Result(); err != nil {
		degrade(domain.SourceHistory)
	} else if write {
		v.NewUser = n == 1
	} else {
		v.NewUser = n == 0
	}
	for _, c := range []struct {
		cmd *redis.Cmd
		dst *bool
	}{{newDevice, &v.NewDevice}, {newBeneficiary, &v.NewBeneficiary}, {newCountry, &v.NewCountry}} {
		if c.cmd == nil {
			continue
		}
		n, err := c.cmd.Int64()
		if err != nil {
			degrade(domain.SourceHistory)
			continue
		}
		*c.dst = n == 1
	}
	if newSender != nil {
		n, err := newSender.Int64()
		if err != nil {
			degrade(domain.SourceHistory)
		} else {
			fresh := n == 1
			v.NewSender = &fresh
		}
	}
	for _, c := range []struct {
		cmd *redis.BoolCmd
		dst **bool
	}{{senderRelated, &v.SenderRelated}, {toPriorSender, &v.ToPriorSender}} {
		if c.cmd == nil {
			continue
		}
		b, err := c.cmd.Result()
		if err != nil {
			degrade(domain.SourceHistory)
			continue
		}
		*c.dst = &b
	}
	if credChange != nil {
		ts, err := credChange.Int64()
		switch {
		case errors.Is(err, redis.Nil):
		case err != nil:
			degrade(domain.SourceHistory)
		default:
			v.RecentCredentialChange = time.Since(time.Unix(ts, 0)) < domain.CredentialChangeWindowHr*time.Hour
		}
	}
	if amount != nil {
		vals, err := amount.Result()
		if err != nil {
			degrade(domain.SourceHistory)
		} else {
			v.AmountCount, v.AmountMean, v.AmountStd = amountStats(vals)
		}
	}
	if accountVel != nil {
		counts, err := accountVel.Int64Slice()
		if err != nil || len(counts) != 4 {
			degrade(domain.SourceVelocity)
		} else {
			v.AccountTxn1m, v.AccountTxn1h, v.AccountTxn24h = counts[0]+current, counts[2]+current, counts[3]+current
		}
	}
	if loginVel != nil {
		counts, err := loginVel.Int64Slice()
		if err != nil || len(counts) != 4 {
			degrade(domain.SourceVelocity)
		} else {
			v.FailedLogins15m = counts[1]
			if !write && e.Login.Outcome == "failure" {
				v.FailedLogins15m++
			}
		}
	}
	if deviceUsers != nil {
		if n, err := deviceUsers.Result(); err != nil {
			degrade(domain.SourceNetwork)
		} else {
			v.DeviceUsers = n
		}
	}
	if fanIn != nil {
		if n, err := fanIn.Result(); err != nil {
			degrade(domain.SourceNetwork)
		} else {
			v.BeneficiarySenders24h = n
		}
	}
	return v
}

// loadScripts makes sure the pipelined scripts are in the Redis script cache. Failure is left
// for the pipeline to report.
func (s *FeatureStore) loadScripts(ctx context.Context) {
	if s.scriptsLoaded.Load() {
		return
	}
	// Not Script.Load: on a pipeline it replaces the script's hash with the not-yet-available
	// reply (an empty string) in go-redis v9.22.
	pipe := s.rdb.Pipeline()
	for _, src := range []string{seenScript, velocityScript, windowScript, profileScript, exportScript} {
		pipe.ScriptLoad(ctx, src)
	}
	if _, err := pipe.Exec(ctx); err == nil {
		s.scriptsLoaded.Store(true)
	}
}

// RecordAmount adds a debit to the user's amount baseline for its currency. Callers exclude
// high-risk events, so an attacker cannot slowly teach the baseline that fraud is normal
// (ARCHITECTURE.md §16.8).
func (s *FeatureStore) RecordAmount(ctx context.Context, e features.Event) error {
	if !e.IsDebit() {
		return nil
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	key := s.keys(scope).entity("u", e.Actor.UserID) + ":amt:" + e.Transaction.Currency
	hour := time.Now().Unix() / 3600
	if err := s.amount.Run(ctx, s.rdb, []string{key}, e.Transaction.Amount, amountBaselineN,
		int(historyTTL.Seconds()), s.baselineMaxPerHour, hour).Err(); err != nil {
		return fmt.Errorf("record amount baseline: %w", err)
	}
	return nil
}

func amountStats(vals []any) (n int64, mean, std float64) {
	parse := func(v any) float64 {
		s, ok := v.(string)
		if !ok {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return f
	}
	n = int64(parse(vals[0]))
	mean = parse(vals[1])
	if n > 1 {
		std = math.Sqrt(math.Max(parse(vals[2]), 0) / float64(n-1))
	}
	return n, mean, std
}

// isCommandError reports whether err is an error reply to an individual command (handled per
// command below) rather than a failure of the whole round trip.
func isCommandError(err error) bool {
	var redisErr redis.Error
	return errors.As(err, &redisErr)
}

func allSources() []string {
	return []string{domain.SourceHistory, domain.SourceVelocity, domain.SourceNetwork}
}

func boolArg(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
