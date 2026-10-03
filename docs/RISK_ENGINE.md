# Risk engine

**Purpose:** how signals are produced and combined into an explainable score, mapped to a risk
level and a decision, and how evaluations stay reproducible. Design:
[ARCHITECTURE.md §10 and §12](ARCHITECTURE.md#10-risk-engine). Status: **built (build step 4),
engine `risk-engine-2.0.0` (five bands since build step 7).** Production rule changes need a
second person's approval, and BLOCK is an opt-in per environment behind shadow evidence (build
step 8). Tenant deny lists block listed identifiers whatever the score (after build step 8).

## Pipeline

```
normalized event
  ─▶ feature store (Redis: one pipelined round trip, bounded by RISKOS_FEATURE_TIMEOUT)
  ─▶ built-in signals + facts
  ─▶ tenant rules (compiled, cached per environment) ─▶ active matches add signals,
                                                       shadow matches are only recorded
  ─▶ score (sum of weights, capped at 100) ─▶ level ─▶ decision policy
  ─▶ immutable evaluation record (same transaction as the event)
```

Code: `backend/internal/risk` (`internal/features` derives signals and facts, `internal/rules`
compiles and evaluates conditions, `internal/engine` combines them, `internal/domain` holds the
score, level and policy, `internal/store` the Redis feature store and the rule tables).

## Built-in signals

Default weights are the spec's (§8) for its five signals; the other five are additions for
account-takeover and mule patterns. Tenants can change any weight between 0 and 50
(`PUT /api/v1/risk/settings`); 0 turns a signal off. No single built-in signal can decide alone.

| Code | Default | Fires when | Source |
|---|---|---|---|
| `new_device` | 20 | The user's device was never seen before | history |
| `new_beneficiary` | 15 | The user never paid this beneficiary before | history |
| `amount_anomaly` | 24 | A debit is above mean + 3σ **and** above 2× the mean of the user's amounts in that currency, with at least 5 prior transactions | history |
| `velocity_anomaly` | 15 | More than 3 transactions from the account in the current minute, or more than 10 in the last hour | velocity |
| `geo_anomaly` | 10 | The event comes from a country never seen for the user | history |
| `failed_login_burst` | 20 | 5 or more failed logins for the user in 15 minutes | velocity |
| `shared_device` | 15 | 5 or more distinct users on the device | network |
| `recent_credential_change` | 20 | A debit within 24 hours of an email, phone or password change | history |
| `beneficiary_fan_in` | 15 | 10 or more distinct users paid the beneficiary in about 24 hours | network |
| `new_user` | 5 | The user has no history | history |

Thresholds are constants of the engine version (`internal/domain/signals.go`); changing one is a
new engine version.

**Cold start:** with no history, every "new X" is trivially true and says nothing, so a user's
first event gets only `new_user` (ARCHITECTURE.md §10). Velocity, network and credential-change
signals still apply.

**Worked example (spec §8):** a user with a history of 200,000 NGN transfers from one device sends
5,000,000 NGN to a new beneficiary, from a new device, from a new country, in a burst:
20 + 15 + 24 + 15 + 10 = **84, High, Step-up** (integration test `TestSpecScenarioScoresHigh`).

## Score

```math
\text{score} = \min\left(100,\ \sum_{s \in \text{triggered}} w_s\right)
```

Every point in the score appears in the response as a listed signal, so any response can be
recomputed from its own signals. (The spec's example of 87 from signals summing to 59 is an
illustration error.)

## Bands

Five bands, shared with the console's design system ([design-system.md](design-system.md)):

| Band | Default scores | Glyph | Default decision |
|---|---|---|---|
| Minimal | 0–19 | 1 bar | Allow |
| Low | 20–49 | 2 bars | Allow (Step-up with a strengthenable signal) |
| Elevated | 50–69 | 3 bars | Review |
| High | 70–89 | 4 bars | Step-up (Review where step-up is impossible) |
| Critical | 90–100 | 5 bars | Review |

The thresholds (20/50/70/90) are per environment (`thresholds` in `PUT /api/v1/risk/settings`,
strictly increasing). `GET /api/v1/risk/policy` returns the environment's bands, thresholds and
default decisions so the console can draw its ruler, and whether they are a custom calibration.
These bands are product defaults, not industry standards, and are never presented as such.

Engine 2.0.0 replaced the four levels of 1.0.0 (low 0–29, medium 30–59, high 60–79, critical
80–100). Migration 00007 recomputed the band of the rows stored before (pre-production data only)
from their stored scores; stored decisions were not changed. Alert severities and case priorities
use the same five bands.

## Decision policy

| Level | Decision | Override (recorded in the evaluation's `policy_reasons`) |
|---|---|---|
| MINIMAL | ALLOW | |
| LOW | ALLOW | STEP_UP when a strengthenable signal fired (`new_device`, `geo_anomaly`, `recent_credential_change`): `strengthenable_signal` |
| ELEVATED | REVIEW | |
| HIGH | STEP_UP | REVIEW for event types that cannot be stepped up (only logins and transactions can): `step_up_not_available` |
| CRITICAL | REVIEW | |
| CRITICAL, automatic blocking in shadow | REVIEW | marked `would_block` |
| CRITICAL, automatic blocking on | BLOCK | `auto_block` |

### Automatic blocking

BLOCK comes only from automatic blocking (ARCHITECTURE.md decision 12). It is set per
environment in the risk settings (`auto_block.mode`) and is off by default.

- **Off.** Critical scores get REVIEW.
- **Shadow.** Critical scores still get REVIEW, but each evaluation is marked `would_block`.
  `GET /api/v1/risk/policy` shows how many evaluations were marked in the last 14 days, since
  when, and from when blocking can be switched on.
- **On.** Critical scores get BLOCK, explained by the same named signals as any other decision.
  It can be switched on only after 14 days in shadow mode (an assumption to confirm with
  customers). Earlier attempts, or attempts straight from off, are refused.
- **Kill switch.** Switching back to shadow or off is always allowed and takes effect at once.
  Off discards the shadow evidence; shadow keeps it.
- Every mode change is audited (`risk.auto_block_changed`), and in the console it needs a recent
  second factor.
- BLOCK decisions raise alerts like REVIEW does.

Decisions stay advisory: the customer's own flow decides what a BLOCK does.

## Deny lists

A deny list holds identifiers a tenant never wants to transact with, per environment. An event
whose user, account, device, IP address (`context.ip`), beneficiary or country is listed is
BLOCKED whatever its score or auto-block mode, with the signal `deny_list` ("On your deny list:
device dev_9a1", weight 0) and the reason `deny_list`. The score is not inflated: the block comes
from the list, and the score still reports the other signals.

- **Matching is exact** after normalisation: IP addresses are canonical (an IPv4-mapped IPv6
  address matches its IPv4 form), countries are two upper-case letters, other identifiers are
  trimmed. There are no ranges, prefixes or patterns.
- **Entries can expire.** An expired entry stops applying; it stays listed, marked expired, until
  someone removes it.
- **Limits.** 10,000 entries per environment; an identifier is listed once per kind.
- **Speed.** The list is part of the cached risk configuration, so a lookup costs nothing on the
  sync path. Adding or removing an entry refreshes the cache at once on the instance that took
  the change, and within 5 seconds on the others.
- **Control.** Adding and removing need `risk:configure`, are audited (`deny_list.added`,
  `deny_list.removed`) and need a recent second factor in the console. Reading needs
  `rules:read` or `risk:configure`. The rule code `deny_list` is reserved.

Decisions stay advisory: the customer's own flow decides what a BLOCK does.

## Degradation

When the feature store cannot answer in time (or a command fails), the signals of the affected
source (history, velocity, network) are listed in `degraded_signals` instead of being guessed, and
rules on the missing facts do not match. The decision then follows the tenant's fail policy per
event type:

- **closed** (default for everything except logins): at least REVIEW (`fail_closed`);
- **open** (default for logins): the decision stands, with the degraded list in the response.

Regardless of policy, while anything is degraded, events at or above the tenant's high-value
threshold (default 1,000,000 minor units, placeholder; per-currency overrides) get at least
REVIEW (`degraded_high_value_floor`). A failure may reduce coverage; it never silently lowers risk.
Every degraded evaluation is logged as a warning and raises a `security` alert in the tenant's
queue ([ALERTS_AND_CASES.md](ALERTS_AND_CASES.md)), since degradation may be attacker-induced.

Tested by pointing the engine at an unreachable Redis (`TestDegradedFeatureStore`) and by
flushing Redis's script cache mid-run (`TestFeatureStoreRecoversFromScriptCacheLoss`).

**History warm-up.** If Redis loses its state, or the feature hash key changes, history signals
that would mislead are held back for a warm-up period and listed in `warming_signals` instead
([DISASTER_RECOVERY.md](DISASTER_RECOVERY.md#losing-redis-state)). Unlike degradation, warming
does not trigger the fail policy.

## Alerts

Ingested decisions at or above the environment's alert threshold (`alert_min_decision`: REVIEW by
default, or STEP_UP) raise `risk` alerts asynchronously; see [ALERTS_AND_CASES.md](ALERTS_AND_CASES.md).

## Rules

Tenants add their own signals as rules (`/api/v1/rules`, scopes `rules:read` / `rules:write`).

- **Conditions** are JSON: `{"all": [...]}`, `{"any": [...]}`, `{"not": ...}` and comparisons
  `{"field", "op", "value"}` over a typed catalog of facts (`GET /api/v1/risk/catalog`): event
  fields (`transaction.amount`, `context.country`, `login.outcome`, ...) and feature values
  (`feature.new_beneficiary`, `feature.account_txn_1h`, `feature.amount_to_mean_ratio`, ...).
  Operators: `eq ne gt gte lt lte in not_in` for numbers, `eq ne in not_in` for strings, `eq ne`
  for booleans.
- **No code:** no `eval`, no scripting, no regular expressions at all. Unknown fields, operators,
  keys or wrongly typed values are rejected with a path to the problem
  (`condition.all[1]: unknown field ...`).
- **Bounds:** depth 6, 50 nodes, 100 list items per `in`, 200 rules per environment (disabled
  rules count). Evaluation is a walk of the compiled tree: linear in its size.
- **A missing fact never matches** (for example a feature whose source is degraded).
- **Versioned:** every change is a new immutable version (`rule_versions` is insert-only);
  rollback creates a new version restoring an earlier one (`restored_from`). A patch that changes
  nothing creates no version. Creation, updates and rollbacks are audited (with the version
  written and, for rollbacks, the version restored).
- **Shadow mode:** a shadow rule is evaluated on live traffic and its matches are stored with the
  evaluation (`shadow_signals`) without affecting score or decision. Start new rules in shadow.
- **Testing:** `POST /api/v1/rules/{id}/test` runs the current version, or a draft condition,
  against an event and returns whether it matched and the facts it saw. Nothing is stored and
  behavioral state is only read, never updated.
- **Codes** are unique per environment and cannot reuse a built-in signal's code; the rule's code
  is the signal code it emits.
- Rules and settings are cached per environment for 5 seconds; the instance that made a change
  applies it at once, other instances within 5 seconds.

### Two-person approval in production

In a production environment, creating, changing or rolling back a rule does not apply it.

- **Proposals.** The API answers `202` with a proposed change (`rch_…`), and the rule keeps its
  current version. A rule, and a new rule's code, can each have one pending change at a time.
- **Approval.** Another person approves it with `POST /api/v1/rule-changes/{id}/approve`:
  - It applies the change as the rule's next version, credited to the proposer.
  - It only succeeds if the rule has not changed since the proposal.
  - Approving needs `rules:approve`, which only people hold (administrators and risk managers).
    An API key can never be granted it, and nobody can approve their own proposal.
  - In the console it also needs a recent second factor.
- **Rejection.** `POST …/reject` rejects a change, with an optional reason. The proposer can
  withdraw their own change the same way.
- **Audit.** Proposals, approvals and rejections are audited (`rule.change_proposed`,
  `rule.change_approved`, `rule.change_rejected`), next to the usual `rule.created`,
  `rule.updated` and `rule.rolled_back` with `change_id` and `proposed_by`.
- **Integrity.** The database refuses to change a proposal's content or a decision once made.
- **Test environments** apply changes at once, for fast iteration.

Rules only add weight; hard blocks come from [deny lists](#deny-lists).

## Rule packs

A workspace with a compliance profile (Compliance → Rules library) is evaluated against its
jurisdiction's rule pack ([ADR-0006](adr/ADR-0006-jurisdiction-rule-packs.md)), after the
built-in signals and tenant rules, in the same evaluation:

```
event ─▶ pack event kinds (named in compliance.kind, or derived)
      ─▶ facts: the event, the customer's compliance.facts, behaviour history
      ─▶ pack windows + activity history (Redis: a second pipelined round trip)
      ─▶ every rule the kinds trigger: fired, clear, not evaluated or not applicable
      ─▶ live signals add weight; live controls and signal floors raise the decision
```

- **Which rules.** Only rules that apply to the profile (licence type and capabilities) and are
  on by default. Regulatory controls run live; risk-scoring rules start in shadow (recorded in
  `shadow_signals`, never scored). Governance presets are workflows, not event rules.
- **Decisions.** A live control sets the action the law requires (`decline`, `hold`,
  `block_session`, `restrict`, `flag`, `fee`, `review`, `block`); a live signal can require at
  least `review` or `step_up`. The most severe of these and the score's decision is returned
  (severity: allow, fee, flag, step_up, review, restrict, hold, block_session, decline, block),
  and each one that reached it is recorded in `policy_reasons` (`control:REG-01`,
  `min_decision:ATO-01`). Every control that applied, live or shadow, is in `controls`, with its
  tasks, fee or hold.
- **Missing data is reported, never read as no risk.** Evaluation is three-valued: a missing
  fact, an unset setting (amounts the institution must choose, such as ATO-01's floor), an
  unavailable window or an amount in another currency than the rule's makes a comparison
  unknown; a rule that cannot be decided is listed in `not_evaluated` with what it lacked. One
  known finding is enough to fire a rule ("any of"), and one known failure to clear it ("all of").
- **Exemptions.** A control whose exemption cannot be checked (REG-04's raised limit) applies,
  and says so (`exemption_unknown`): an unknown exemption never lifts a legal limit.
- **Amounts** are compared in one currency only: there is no conversion on the hot path. Rules
  on US dollar amounts read `txn.amount_usd`, sent by the customer from its own rates.

### Configuration

Each environment can set a rule's mode (`live`, `shadow`, `off`) and its settings
(`PUT /api/v1/compliance/rules/{rule_id}/config`, people with `compliance:configure`, a recent
second factor in the console):

- **Bounds.** Every setting stays within the pack's minimum and maximum; for regulatory limits
  the maximum is the legal one. Money, counts, durations, hours and points are whole numbers. A
  value equal to the default is not stored, so "back to the default" is one state.
- **Versions.** Every change is a new immutable version with its reason and author; version 0 is
  the library's defaults. A rollback restores an earlier version as the next one.
- **Two people in production.** For a regulatory control, any settings change and any move away
  from `live` is proposed (`202`, `pch_…`) and applies only when another person with
  `rules:approve` approves it, credited to the proposer, and only if the rule has not changed
  since (`409` otherwise). Whether a setting loosens depends on how the rule uses it, so every
  settings change is checked. Switching a control back to `live`, and any change in the test
  environment or to a risk-scoring rule, applies at once. A rule has at most one pending change;
  the proposer can withdraw it.
- **Audit.** `pack_rule.configured`, `pack_rule.change_proposed`, `pack_rule.change_approved`
  and `pack_rule.change_rejected`, with the settings and mode before and after.
- **Effect.** The engine runs each rule with its environment's configuration; other API
  instances pick a change up within 5 seconds (the risk configuration cache).

### Backtests

`POST /api/v1/compliance/rules/{rule_id}/backtest` (`compliance:read` and `events:read`) answers
"what would this rule have done?": it replays the environment's ingested events of the last 1 to
30 days (at most 20,000, oldest first) through the rule with candidate settings and with its
current configuration, side by side, whatever its mode. Time windows are rebuilt in memory with
the same rules as the Redis windows, from a replay record each evaluation keeps (event time,
amount, and the entities and identifiers windows use, hashed). Nothing is stored and live
windows are untouched; it is one batch read, never a query per event. Events evaluated before an
evaluation kept a replay record are not replayed, and the response says how many were.

### Pack windows

Windowed aggregates (counts, sums, daily averages, distinct counts) are kept in Redis, never
queried from Postgres per event:

- One sorted set per window definition and entity (`rk:<tenant>:<env>:{w:<entity kind>:<hmac>}:<key>`),
  scored by the event's `occurred_at`, trimmed to the longest window the rule's setting allows
  and to 20,000 entries. Identifiers and event IDs are HMAC'd like every feature-store key.
- A window's key covers the events it holds, the entity, the identifier it counts and its filter
  with the filter's settings: aggregates that differ only by function or length share a window,
  and changing a filter's setting starts a fresh one.
- An event is recorded in every window of the pack over one of its kinds whose filter it passes.
  An event whose filter cannot be checked (a fact it needs is missing) is recorded as unknown, and
  a window holding one reads as unavailable for as long as it holds it, rather than undercounting.
- Sums count only amounts in the event's currency. Dry runs and rule tests count the event
  without storing it; idempotent replays are not evaluated again.
- While behavioural history warms up after a reset, windows and history facts are unavailable,
  so the rules on them are not evaluated.
- The same round trip keeps each user's first activity, each account's last activity (400 days)
  and each user's largest amount per currency: `customer.history_age`, `account.inactive_for` and
  `history.amount_to_max`.

### History facts not tracked yet

`history.hour_usual`, `geo.speed_from_previous`, `flows.prior_sender_outflow_share`,
`sender.related` and `export.rows_to_*_baseline` need behaviour RiskOS does not record yet. The
rules on them (BEH-02's impossible-travel branch, BEH-03, PON-01, SEC-12) are reported as not
evaluated until it does; [the tracker](compliance/TRACKER.md) names the phase that adds each.

### Reproducibility

The evaluation records the pack version (`pack:ng@2026.10.0` in `rule_versions`), the event's
kinds, the facts the pack saw, and the values each fired rule compared, next to `controls` and
`not_evaluated`.

### Cost

`BenchmarkOutboundTransfer` (the 34 rules an outbound transfer triggers in the Nigeria pack, with
windows already read): about 66 µs mean and 125 µs p95 per event on a laptop. The decision path's
end-to-end p95 with the pack is measured in the hardening phase.

## Feature store

Redis, one pipelined round trip per evaluation (plus one write for the amount baseline after a
non-high-risk debit):

- **Seen sets** (devices, beneficiaries, countries per user, capped at 200 members, 180 days).
- **Velocity:** a Lua script keeps 1-minute and 1-hour buckets on the Redis clock and returns
  current-minute, 15-minute, 60-minute and 24-hour counts.
- **Network:** HyperLogLog distinct counts of users per device and per beneficiary per day.
- **Amount baseline:** a running mean and variance per user and currency (Welford), weighted
  towards the last ~500 transactions.
- **Anti-poisoning:** HIGH and CRITICAL events never update the amount baseline (§16.8).
- **Privacy:** identifiers never reach Redis in the clear. Keys are
  `rk:<tenant>:<environment>:{<kind>:<hmac>}`, with an HMAC keyed by `RISKOS_FEATURE_HASH_KEY`
  (at least 32 bytes). The braces are the Redis Cluster hash tag. Rotating the key resets history.
- Scripts are sent by SHA (`EVALSHA`) after a one-time `SCRIPT LOAD`, which cut Redis CPU per
  script call from about 100 µs to about 20 µs; a flushed script cache degrades at most one
  evaluation and reloads.
- **Dry runs and rule tests only read.** Ingested events update state; replays of an idempotent
  event do not (they return the stored response).

## Money

Integer minor units plus an ISO 4217 currency. Baselines and high-value thresholds are per
currency; there is no FX conversion on the hot path.

## Reproducibility

Every evaluation stores the engine version, the versions of the rules evaluated
(`code@version`), the facts used, the signals (and shadow signals), the policy reasons, the
decision and its duration. Dry runs also store the normalized event; an ingested event is in
`events`. Evaluations are insert-only.

## Performance

See [PERFORMANCE_BASELINE.md](PERFORMANCE_BASELINE.md) §4: at 100 events/s the decision path's
p95 is about 13 ms on a laptop.
