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

// windowScript records an event in the window KEYS[1] (write mode, with a member) and computes
// aggregates over [now - window, now] for each query. A dry run counts its own member without
// storing it. Members are "<id>|<flag>|<field>|<per>|<amount>|<currency>"; flag "?" marks an event
// whose filter could not be checked, which makes every result of the window unknown ("?").
// Amounts are summed in the event's currency only.
const windowScript = `
local now = tonumber(ARGV[3])
local member = ARGV[2]
if ARGV[1] == '1' and member ~= '' then
  local retention = tonumber(ARGV[4])
  redis.call('ZADD', KEYS[1], now, member)
  redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', '(' .. (now - retention))
  local n = redis.call('ZCARD', KEYS[1])
  local cap = tonumber(ARGV[5])
  if n > cap then redis.call('ZREMRANGEBYRANK', KEYS[1], 0, n - cap - 1) end
  redis.call('PEXPIRE', KEYS[1], retention + tonumber(ARGV[6]))
end
local cur = ARGV[7]
local out = {}
for q = 0, tonumber(ARGV[8]) - 1 do
  local fn = ARGV[9 + 2 * q]
  local win = tonumber(ARGV[10 + 2 * q])
  local ms = redis.call('ZRANGEBYSCORE', KEYS[1], now - win, now)
  if ARGV[1] ~= '1' and member ~= '' then ms[#ms + 1] = member end
  local unknown, count, sum = false, 0, 0
  local fields, per = {}, {}
  for _, m in ipairs(ms) do
    local _, flag, field, pv, amt, c = string.match(m, '^([^|]*)|([^|]*)|([^|]*)|([^|]*)|([^|]*)|([^|]*)$')
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
  local v
  if unknown then
    v = '?'
  elseif fn == 'count' then
    v = count
  elseif fn == 'exists' then
    if count > 0 then v = 1 else v = 0 end
  elseif fn == 'sum' then
    v = sum
  elseif fn == 'daily_avg' then
    v = sum / (win / 86400000)
  elseif fn == 'distinct_count' or fn == 'distinct_count_once' then
    v = 0
    for _, n in pairs(fields) do
      if fn == 'distinct_count' or n == 1 then v = v + 1 end
    end
  elseif fn == 'max_distinct_per_value' then
    v = 0
    for _, set in pairs(per) do
      local d = 0
      for _ in pairs(set) do d = d + 1 end
      if d > v then v = d end
    end
  else
    v = '?'
  end
  out[#out + 1] = string.format('%s', v)
end
return out
`

// profileScript keeps first activity per user (KEYS[1]), last activity per account (KEYS[2]) and
// the largest amount per user and currency, all by event time. Returns the user's first activity,
// the account's previous activity and the previous largest amount ("" when unknown).
const profileScript = `
local now = tonumber(ARGV[2])
local first = redis.call('HGET', KEYS[1], 'first')
local last = ''
if KEYS[2] ~= KEYS[1] then last = redis.call('HGET', KEYS[2], 'last') or '' end
local maxf = 'max:' .. ARGV[4]
local max = ''
if ARGV[4] ~= '' then max = redis.call('HGET', KEYS[1], maxf) or '' end
if ARGV[1] == '1' then
  if not first then redis.call('HSET', KEYS[1], 'first', now) end
  if KEYS[2] ~= KEYS[1] and (last == '' or tonumber(last) < now) then redis.call('HSET', KEYS[2], 'last', now) end
  if ARGV[4] ~= '' and (max == '' or tonumber(max) < tonumber(ARGV[3])) then redis.call('HSET', KEYS[1], maxf, ARGV[3]) end
  redis.call('PEXPIRE', KEYS[1], ARGV[5])
  if KEYS[2] ~= KEYS[1] then redis.call('PEXPIRE', KEYS[2], ARGV[5]) end
end
return {first or '', last, max}
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
		cmds[i] = s.window.EvalSha(ctx, pipe, []string{key}, args...)
	}
	user := k.entity("u", e.UserID) + ":pf"
	account := user
	if e.AccountID != "" {
		account = k.entity("a", e.AccountID) + ":pf"
	}
	prof := s.profile.EvalSha(ctx, pipe, []string{user, account}, boolArg(write), at, e.Amount, e.Currency, profileTTL.Milliseconds())
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) && !isCommandError(err) {
		return nil, features.Activity{}, false
	}
	for _, cmd := range append(cmds, prof) {
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
	if p, err := prof.StringSlice(); err == nil && len(p) == 3 {
		profile = parseProfile(p)
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
	return out
}
