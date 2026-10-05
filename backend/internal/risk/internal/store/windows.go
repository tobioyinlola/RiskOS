package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
)

// Rule pack windows (ADR-0006): one sorted set per window key and entity, scored by event time,
// trimmed to the longest window the rule's setting allows. Reads and writes for an event go in
// one pipelined round trip, after the behavioral features (whose facts decide what is recorded).

// Bounds of pack window state.
const (
	// windowCap bounds the entries of one window (the largest count a pack setting allows is
	// 10,000 accounts; the cap leaves headroom). Beyond it the oldest entries go first.
	windowCap = 20_000
	// windowGrace keeps a window a day beyond its retention, so expiry never races a read.
	windowGrace = 24 * time.Hour
	// profileTTL keeps first/last activity long enough to see a year of dormancy.
	profileTTL = 400 * 24 * time.Hour
)

// windowScript records an event in a window (write mode, with a member) and computes aggregates
// over [now - window, now] for each query. A dry run counts its own member without storing it.
//
// KEYS[1] is a sorted set of the window's events by time. Members are
// "<id>|<flag>|<field>|<per>|<amount>|<currency>"; flag "?" marks an event whose filter could not
// be checked, which makes every result of the window unknown ("?"). Amounts are summed in the
// event's currency only.
//
// Reads must not scan the whole window: a busy entity (a biller paid by thousands of customers)
// would cost every event milliseconds of Redis time, shared by every tenant. So the script keeps,
// next to the sorted set, the unknown events by time in KEYS[2] (one ZCOUNT says whether a window
// is unknown), each counted field's latest time in KEYS[3] (a distinct count is one ZCOUNT), and
// in KEYS[4] the count and the sum per currency of each time bucket (about 1,024 per retention).
// Counts and exists are one ZCOUNT (no unknown entry means every entry counts). Sums and daily
// averages add whole buckets and scan only the two partial buckets at the window's edges, once
// per window length, unless the window holds fewer entries than its buckets would cost to fetch:
// then it is scanned. An empty window answers at once. The bucket size follows the retention a write
// gives (an event that only reads a window gives none, so its longest query stands in), and a
// write with another retention reindexes the window; a query over more than 4,096 buckets scans
// instead. A write costs a few commands: trimming, dropping old buckets and refreshing the
// indexes' expiry happen once per bucket. Concurrent events reach Redis slightly out of order, so an event at most a minute
// older than the newest one counts the identifiers seen up to the newest; an older one,
// and the rarer distinct_count_once and max_distinct_per_value, scan the window. Every result is
// exact, except that a distinct count may include identifiers first seen in that minute after
// the event, or seen only in entries evicted beyond the cap: more risk, never less. A window written before these structures existed is
// indexed on first use.
const windowScript = `
local K, KU, KF, KB = KEYS[1], KEYS[2], KEYS[3], KEYS[4]
local write = ARGV[1] == '1'
local member = ARGV[2]
local now = tonumber(ARGV[3])
local retention = tonumber(ARGV[4])
local cap = tonumber(ARGV[5])
local ttl = retention + tonumber(ARGV[6])
local cur = ARGV[7]
-- How much older than the newest entry an event may be for its distinct counts to use the index.
local skew = 60000

local function parse(m)
  return string.match(m, '^([^|]*)|([^|]*)|([^|]*)|([^|]*)|([^|]*)|([^|]*)$')
end

local dry = (not write) and member ~= ''
local _, dflag, dfield, _, damt, dc
if dry then _, dflag, dfield, _, damt, dc = parse(member) end

-- days_over counts the local days of the window whose total in the event's currency reached a
-- threshold. fn is "days_over:<threshold>:<offset ms>", the offset moving UTC to the local day's
-- start (ADR-0007).
local function days_over(fn, win)
  local threshold, offset = string.match(fn, '^days_over:(%-?%d+):(%-?%d+)$')
  threshold, offset = tonumber(threshold), tonumber(offset)
  local ms = redis.call('ZRANGEBYSCORE', K, now - win, now, 'WITHSCORES')
  local totals = {}
  local function add(flag, amt, c, t)
    if flag == '1' and c == cur and amt ~= '' then
      local d = math.floor((t + offset) / 86400000)
      totals[d] = (totals[d] or 0) + tonumber(amt)
    end
  end
  for i = 1, #ms, 2 do
    local _, flag, _, _, amt, c = parse(ms[i])
    if flag == '?' then return '?' end
    add(flag, amt, c, tonumber(ms[i + 1]))
  end
  if dry then
    if dflag == '?' then return '?' end
    add(dflag, damt, dc, now)
  end
  local n = 0
  for _, s in pairs(totals) do
    if s >= threshold then n = n + 1 end
  end
  return n
end

-- scan computes fn over the whole window, member by member.
local function scan(fn, win)
  if string.sub(fn, 1, 10) == 'days_over:' then return days_over(fn, win) end
  local ms = redis.call('ZRANGEBYSCORE', K, now - win, now)
  if dry then ms[#ms + 1] = member end
  local unknown, count, sum = false, 0, 0
  local fields, per = {}, {}
  for _, m in ipairs(ms) do
    local _, flag, field, pv, amt, c = parse(m)
    if flag == '?' then
      unknown = true
    elseif flag == '1' then
      count = count + 1
      if c == cur and amt ~= '' then sum = sum + tonumber(amt) end
      if field ~= '' then
        fields[field] = (fields[field] or 0) + 1
        if pv ~= '' then
          per[pv] = per[pv] or {}
          per[pv][field] = true
        end
      end
    end
  end
  if unknown then return '?' end
  if fn == 'count' then return count end
  if fn == 'exists' then if count > 0 then return 1 end return 0 end
  if fn == 'sum' then return sum end
  if fn == 'daily_avg' then return sum / (win / 86400000) end
  if fn == 'distinct_count' or fn == 'distinct_count_once' then
    local v = 0
    for _, n in pairs(fields) do
      if fn == 'distinct_count' or n == 1 then v = v + 1 end
    end
    return v
  end
  if fn == 'max_distinct_per_value' then
    local v = 0
    for _, set in pairs(per) do
      local d = 0
      for _ in pairs(set) do d = d + 1 end
      if d > v then v = d end
    end
    return v
  end
  return '?'
end

local writing = write and member ~= ''
local entries = redis.call('ZCARD', K)
-- Most windows an event reads are empty for its entity: answer them without touching the indexes.
if not writing and entries == 0 then
  local out = {}
  for q = 0, tonumber(ARGV[8]) - 1 do
    out[#out + 1] = string.format('%s', scan(ARGV[9 + 2 * q], tonumber(ARGV[10 + 2 * q])))
  end
  return out
end

local meta = redis.call('HMGET', KB, 'B', 'last')
local B, last = tonumber(meta[1] or ''), tonumber(meta[2] or '')
local function bucket(t) return math.floor(t / B) end
local function size(r) return math.max(1000, math.ceil(r / 1024)) end
local longest = 0
for q = 0, tonumber(ARGV[8]) - 1 do longest = math.max(longest, tonumber(ARGV[10 + 2 * q])) end

-- apply adds (sign 1) or removes (sign -1) a member's share of the aggregates.
local function apply(m, t, sign)
  local id, flag, field, _, amt, c = parse(m)
  if flag == '?' then
    if sign > 0 then redis.call('ZADD', KU, t, id) else redis.call('ZREM', KU, id) end
  elseif flag == '1' then
    local b = bucket(t)
    redis.call('HINCRBY', KB, 'c' .. b, sign)
    if amt ~= '' then
      if sign < 0 then amt = '-' .. amt end
      redis.call('HINCRBY', KB, 's' .. b .. ':' .. c, amt)
    end
    if sign > 0 and field ~= '' then redis.call('ZADD', KF, 'GT', t, field) end
  end
end

-- reindex rebuilds the indexes from the window with bucket size b.
local function reindex(b)
  redis.call('DEL', KU, KF, KB)
  B, last = b, nil
  if entries == 0 then return end
  redis.call('HSET', KB, 'B', B)
  local all = redis.call('ZRANGE', K, 0, -1, 'WITHSCORES')
  for i = 1, #all, 2 do apply(all[i], tonumber(all[i + 1]), 1) end
  for _, k in ipairs({KU, KF, KB}) do redis.call('PEXPIRE', k, ttl) end
end

if writing and B ~= size(retention) then
  reindex(size(retention))
elseif not B then
  reindex(size(math.max(retention, longest)))
end

if writing then
  if redis.call('ZADD', K, 'NX', now, member) == 1 then
    entries = entries + 1
    apply(member, now, 1)
  end
  redis.call('PEXPIRE', K, ttl)
  if entries > cap then
    local evicted = redis.call('ZRANGE', K, 0, entries - cap - 1, 'WITHSCORES')
    for i = 1, #evicted, 2 do apply(evicted[i], tonumber(evicted[i + 1]), -1) end
    redis.call('ZREMRANGEBYRANK', K, 0, entries - cap - 1)
    entries = cap
  end
  -- Once per new bucket: trim to the retention, drop older buckets and refresh the indexes'
  -- expiry (missing indexes are rebuilt from the window, so they may expire first).
  local nb = bucket(now)
  if not last or nb > last then
    local oldest = now - retention
    for _, k in ipairs({K, KU, KF}) do redis.call('ZREMRANGEBYSCORE', k, '-inf', '(' .. oldest) end
    redis.call('HSET', KB, 'B', B, 'last', nb)
    local keep = bucket(oldest) - 1
    for _, f in ipairs(redis.call('HKEYS', KB)) do
      local b = tonumber(string.match(f, '^[cs](%d+)'))
      if b and b < keep then redis.call('HDEL', KB, f) end
    end
    for _, k in ipairs({KU, KF, KB}) do redis.call('PEXPIRE', k, ttl) end
    entries = redis.call('ZCARD', K)
  end
end

local top = redis.call('ZRANGE', K, -1, -1, 'WITHSCORES')
local lag = 0
if #top > 0 then lag = tonumber(top[2]) - now end

-- totals returns the count and the sum of a window from the buckets and its edges, once per
-- window length.
local totals = {}
local function total(win)
  if totals[win] then return totals[win][1], totals[win][2] end
  local lo, hi = now - win, now
  local count, sum = 0, 0
  local function add(m)
    local _, flag, _, _, amt, c = parse(m)
    if flag == '1' then
      count = count + 1
      if c == cur and amt ~= '' then sum = sum + tonumber(amt) end
    end
  end
  local bl, bh = bucket(lo), bucket(hi)
  -- Scan when the window holds fewer entries than the buckets would cost to fetch.
  local n = 0
  if entries > 0 then n = redis.call('ZCOUNT', K, lo, hi) end
  if n == 0 then
    -- Nothing stored in the window: only a dry run's own event counts.
  elseif bl == bh or bh - bl > 4096 or n <= 2 * (bh - bl) + 64 then
    for _, m in ipairs(redis.call('ZRANGEBYSCORE', K, lo, hi)) do add(m) end
  else
    for _, m in ipairs(redis.call('ZRANGEBYSCORE', K, lo, (bl + 1) * B - 1)) do add(m) end
    for _, m in ipairs(redis.call('ZRANGEBYSCORE', K, bh * B, hi)) do add(m) end
    if bh - bl > 1 then
      local f = {}
      for b = bl + 1, bh - 1 do
        f[#f + 1] = 'c' .. b
        f[#f + 1] = 's' .. b .. ':' .. cur
      end
      local v = redis.call('HMGET', KB, unpack(f))
      for i = 1, #v, 2 do
        count = count + tonumber(v[i] or 0)
        sum = sum + tonumber(v[i + 1] or 0)
      end
    end
  end
  if dry then add(member) end
  totals[win] = {count, sum}
  return count, sum
end

-- fast computes count, exists, sum, daily_avg and (in order) distinct_count from the indexes.
local function fast(fn, win)
  local lo, hi = now - win, now
  if (dry and dflag == '?') or redis.call('ZCOUNT', KU, lo, hi) > 0 then return '?' end
  if fn == 'distinct_count' then
    local v = redis.call('ZCOUNT', KF, lo, '+inf')
    if dry and dflag == '1' and dfield ~= '' then
      local p = redis.call('ZSCORE', KF, dfield)
      if not p or tonumber(p) < lo then v = v + 1 end
    end
    return v
  end
  if fn == 'count' or fn == 'exists' then
    -- With no unknown entry in the window, every entry counts: one ZCOUNT.
    local count = redis.call('ZCOUNT', K, lo, hi)
    if dry and dflag == '1' then count = count + 1 end
    if fn == 'count' then return count end
    if count > 0 then return 1 end
    return 0
  end
  local _, sum = total(win)
  if fn == 'sum' then return sum end
  return sum / (win / 86400000)
end

local out = {}
for q = 0, tonumber(ARGV[8]) - 1 do
  local fn = ARGV[9 + 2 * q]
  local win = tonumber(ARGV[10 + 2 * q])
  local v
  if fn == 'count' or fn == 'exists' or fn == 'sum' or fn == 'daily_avg' or (fn == 'distinct_count' and lag <= skew) then
    v = fast(fn, win)
  else
    v = scan(fn, win)
  end
  out[#out + 1] = string.format('%s', v)
end
return out
`

// profileScript keeps first activity per user (KEYS[1]), last activity per account (KEYS[2]), the
// largest amount per user and currency, and the hours of the day (ARGV[6], the governing jurisdiction's time) the user
// was active in as a 24-bit mask, all by event time. Returns the user's first activity, the
// account's previous activity, the previous largest amount, the previous hour mask and the
// previous location and its time ("" when unknown).
const profileScript = `
local now = tonumber(ARGV[2])
local first = redis.call('HGET', KEYS[1], 'first')
local last = ''
if KEYS[2] ~= KEYS[1] then last = redis.call('HGET', KEYS[2], 'last') or '' end
local maxf = 'max:' .. ARGV[4]
local max = ''
if ARGV[4] ~= '' then max = redis.call('HGET', KEYS[1], maxf) or '' end
local hours = redis.call('HGET', KEYS[1], 'hours') or ''
local geo = redis.call('HMGET', KEYS[1], 'lat', 'lon', 'geo_at')
if ARGV[1] == '1' and ARGV[7] ~= '' and (not geo[3] or tonumber(geo[3]) <= now) then
  redis.call('HSET', KEYS[1], 'lat', ARGV[7], 'lon', ARGV[8], 'geo_at', now)
end
if ARGV[1] == '1' then
  if ARGV[6] ~= '' then
    local mask = tonumber(hours ~= '' and hours or '0')
    redis.call('HSET', KEYS[1], 'hours', bit.bor(mask, bit.lshift(1, tonumber(ARGV[6]))))
  end
  if not first then redis.call('HSET', KEYS[1], 'first', now) end
  if KEYS[2] ~= KEYS[1] and (last == '' or tonumber(last) < now) then redis.call('HSET', KEYS[2], 'last', now) end
  if ARGV[4] ~= '' and (max == '' or tonumber(max) < tonumber(ARGV[3])) then redis.call('HSET', KEYS[1], maxf, ARGV[3]) end
  redis.call('PEXPIRE', KEYS[1], ARGV[5])
  if KEYS[2] ~= KEYS[1] then redis.call('PEXPIRE', KEYS[2], ARGV[5]) end
end
return {first or '', last, max, hours, geo[1] or '', geo[2] or '', geo[3] or ''}
`

// exportScript keeps a running mean of rows per export for a user (KEYS[1]) and an environment
// (KEYS[2]), over about the last ARGV[3] exports, and returns each mean and count before this
// export.
const exportScript = `
local out = {}
for i = 1, 2 do
  local n = tonumber(redis.call('HGET', KEYS[i], 'n') or '0')
  local mean = tonumber(redis.call('HGET', KEYS[i], 'mean') or '0')
  out[#out + 1] = tostring(n)
  out[#out + 1] = string.format('%.6f', mean)
  if ARGV[1] == '1' then
    local cap = tonumber(ARGV[3])
    if n >= cap then n = cap - 1 end
    n = n + 1
    mean = mean + (tonumber(ARGV[2]) - mean) / n
    redis.call('HSET', KEYS[i], 'n', n, 'mean', string.format('%.6f', mean))
    redis.call('PEXPIRE', KEYS[i], ARGV[4])
  end
end
return out
`

// Windows records the event in, and reads, the given windows, and reads (and in write mode
// updates) the activity profile, in one pipelined round trip bounded by the store timeout.
// values[i][j] answers ops[i].Queries[j]; ok is false when the round trip failed.
func (s *FeatureStore) Windows(ctx context.Context, ops []features.WindowOp, e features.WindowEvent, write bool) (values [][]features.WindowValue, profile features.Activity, ok bool) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, features.Activity{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	s.loadScripts(ctx)
	k := s.keys(scope)
	at := e.At.UnixMilli()
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.Cmd, len(ops))
	for i, op := range ops {
		member := ""
		if op.Record {
			flag := "1"
			if op.Unknown {
				flag = "?"
			}
			field := ""
			if op.Field != "" {
				field = k.hash("f", op.Field)
			}
			member = strings.Join([]string{k.hash("e", e.ID), flag, field, op.Per, strconv.FormatInt(e.Amount, 10), e.Currency}, "|")
		}
		args := []any{
			boolArg(write), member, at, op.Retention.Milliseconds(), windowCap, windowGrace.Milliseconds(),
			e.Currency, len(op.Queries),
		}
		for _, q := range op.Queries {
			args = append(args, q.Fn, q.Window.Milliseconds())
		}
		// The entity's hash tag comes first, so one entity's windows share a Redis Cluster slot.
		key := k.entity("w:"+op.EntityKind, op.EntityID) + ":" + op.Key
		cmds[i] = s.window.EvalSha(ctx, pipe, []string{key, key + ":u", key + ":f", key + ":b"}, args...)
	}
	user := k.entity("u", e.UserID) + ":pf"
	account := user
	if e.AccountID != "" {
		account = k.entity("a", e.AccountID) + ":pf"
	}
	hour := ""
	if e.Hour >= 0 {
		hour = strconv.Itoa(e.Hour)
	}
	lat, lon := "", ""
	if e.Geo != nil {
		lat, lon = strconv.FormatFloat(e.Geo.Lat, 'f', 2, 64), strconv.FormatFloat(e.Geo.Lon, 'f', 2, 64)
	}
	prof := s.profile.EvalSha(ctx, pipe, []string{user, account}, boolArg(write), at, e.Amount, e.Currency,
		profileTTL.Milliseconds(), hour, lat, lon)
	var exp *redis.Cmd
	if e.ExportRows != nil {
		exp = s.export.EvalSha(ctx, pipe, []string{k.entity("u", e.UserID) + ":exp", k.entity("n", "*") + ":exp"},
			boolArg(write), *e.ExportRows, amountBaselineN, historyTTL.Milliseconds())
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) && !isCommandError(err) {
		return nil, features.Activity{}, false
	}
	for _, cmd := range append(cmds, prof, exp) {
		if cmd == nil {
			continue
		}
		if redis.HasErrorPrefix(cmd.Err(), "NOSCRIPT") {
			s.scriptsLoaded.Store(false)
			break
		}
	}
	values = make([][]features.WindowValue, len(ops))
	for i, cmd := range cmds {
		values[i] = make([]features.WindowValue, len(ops[i].Queries))
		out, err := cmd.StringSlice()
		if err != nil || len(out) != len(ops[i].Queries) {
			continue
		}
		for j, raw := range out {
			if v, err := strconv.ParseFloat(raw, 64); err == nil {
				values[i][j] = features.WindowValue{Value: v, OK: true}
			}
		}
	}
	if p, err := prof.StringSlice(); err == nil && len(p) == 7 {
		profile = parseProfile(p)
	}
	if exp != nil {
		if b, err := exp.StringSlice(); err == nil && len(b) == 4 {
			profile.ExportUser, profile.ExportTenant = parseBaseline(b[0], b[1]), parseBaseline(b[2], b[3])
		}
	}
	return values, profile, true
}

func parseProfile(p []string) features.Activity {
	var out features.Activity
	ms := func(s string) *time.Time {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil
		}
		t := time.UnixMilli(n).UTC()
		return &t
	}
	out.UserFirst, out.AccountLast = ms(p[0]), ms(p[1])
	if f, err := strconv.ParseFloat(p[2], 64); err == nil {
		out.MaxAmount = &f
	}
	if h, err := strconv.ParseInt(p[3], 10, 64); err == nil {
		out.Hours = &h
	}
	lat, errLat := strconv.ParseFloat(p[4], 64)
	lon, errLon := strconv.ParseFloat(p[5], 64)
	if at := ms(p[6]); errLat == nil && errLon == nil && at != nil {
		out.LastGeo, out.LastGeoAt = &features.Geo{Lat: lat, Lon: lon}, at
	}
	return out
}

func parseBaseline(n, mean string) features.Baseline {
	c, _ := strconv.ParseInt(n, 10, 64)
	m, _ := strconv.ParseFloat(mean, 64)
	return features.Baseline{Count: c, Mean: m}
}
