# Disaster recovery

**Purpose:** what RiskOS does when a dependency fails, how data is backed up and restored, and
the evidence for both (spec §15; ARCHITECTURE.md §12 failure modes and §14 backup and
recovery). Two drills produce the evidence and can be rerun at any time against the local stack:
`make failure-drill` and `make restore-drill`.

## Recovery objectives

| Data | Where | Objective | Status |
|---|---|---|---|
| System of record: tenants, keys, users, events, evaluations, rules, alerts, cases, webhooks, audit log | PostgreSQL | RPO 5 minutes, RTO 1 hour (proposed targets, to confirm; ARCHITECTURE.md §14) | Restore procedure proven by the restore drill. Point-in-time recovery comes with the managed database. TODO(phase-8) |
| Behavioural state: velocity counters, amount baselines, "seen" sets, rate limits | Redis, with an append-only file on its volume | Survives restarts; not backed up | See [Losing Redis state](#losing-redis-state) |
| Queued work: alert raising, webhook delivery | PostgreSQL (River jobs, written in the same transactions as the data) | Same as PostgreSQL | Nothing is queued anywhere else |

## How RiskOS behaves when a dependency fails

Measured by the failure drill below. Clients follow [API.md § Retries](API.md#retries).

| Failure | Decision path (`POST /api/v1/events`) | Background work | Recovery |
|---|---|---|---|
| Redis down | Decisions continue. See [Redis down](#redis-down) below | Unaffected | Automatic on the first successful call |
| PostgreSQL down | 503 `SERVICE_UNAVAILABLE` with `Retry-After: 1`, within about a second (`RISKOS_POSTGRES_ACQUIRE_TIMEOUT`). Nothing is stored, so a retry with the same `event_id` is safe | Jobs wait. Nothing is lost, because they live in PostgreSQL | Automatic: the pool reconnects |
| Worker killed (SIGKILL) | Unaffected | Queued jobs wait for a worker. Jobs it was running are retried once they have run longer than any job may (`RISKOS_WORKER_JOB_TIMEOUT` plus 1 minute). Alert raising is idempotent per evaluation, and webhook receivers deduplicate on `RiskOS-Webhook-Id` | Restart the worker |
| API restarted (SIGTERM) | In-flight requests finish during the drain. New connections are refused until it is back | Unaffected | Run several instances behind a load balancer (ARCHITECTURE.md §2) |

### Redis down

- History, velocity and network signals are reported in `degraded_signals`.
- Transactions fail closed: the decision is at least REVIEW.
- Every degraded evaluation raises a `security` alert.
- Rate limits fail open.
- After 5 consecutive connectivity failures, a circuit breaker fails Redis calls at once for
  `RISKOS_REDIS_BREAKER_COOLDOWN` (1 s), so requests don't wait on Redis.

## Failure drill

`make failure-drill` (`scripts/failure-drill.sh`, `backend/test/load/failure.js`) runs steady
transaction ingest with k6. Each simulated client retries with the same `event_id`, honours
`Retry-After` and adds jitter.

While the load runs, the drill:
- stops Redis for 30 s;
- stops PostgreSQL for 15 s;
- kills the worker for 30 s;
- restarts the API.

A webhook receiver on the compose network verifies every signature. Once the background work has
drained, the drill checks against the database that:

- no event was lost, and every accepted event is stored exactly once;
- no degraded transaction was allowed;
- every evaluation that should raise an alert did, exactly once per kind;
- no background job was discarded;
- every webhook was delivered, received and correctly signed.

Results on 2026-10-01, 80 events/s for 4 minutes. The machine was a laptop (8 vCPU, WSL2) running
every service.

| | First run | After the fixes below |
|---|---|---|
| Events accepted / stored / lost | 18,487 / 18,487 / 0 | 18,543 / 18,543 / 0 |
| Degraded decisions / degraded and allowed | 2,467 / 0 | 2,730 / 0 |
| Alerts expected / raised | 4,935 / 4,935 | 5,460 / 5,460 |
| Webhooks delivered / received / bad signatures | 368 / 368 / 0 | 308 / 308 / 0 |
| Failed attempts, each retried by its client | 500: 141, 503: 15, 429: 6,550, no response: 721 | 503: 3,797, 429: 5,787, no response: 243 |
| Request latency p50 / p99 / max | 13.5 ms / 10.0 s / 10.0 s | 15.1 ms / 1.04 s / 2.2 s |

The first run found two problems, now fixed:

- **Redis outages stalled requests.**
  - Cause: the rate-limit check used the request's 10 s context, and each call to the stopped
    Redis waited on a DNS lookup or a dial. Requests piled up, held database connections and
    timed out.
  - Fix: rate-limit checks are bounded at 50 ms (`RISKOS_RATE_LIMIT_TIMEOUT`), and the Redis
    client has a circuit breaker.
- **PostgreSQL outages returned 500 and hung.**
  - Cause: a lost or refused connection was treated as an unknown error, and acquiring a
    connection had no deadline of its own.
  - Fix: transactions and the pre-authentication lookups get a connection within 1 s or fail
    with 503 and `Retry-After`.

The remaining failed attempts in the second run are expected:
- the 503s are fast failures during the PostgreSQL outage, each retried;
- the 429s are the per-key limit (100/s locally) pacing the backlog of retries that followed;
- the attempts with no response are refused connections while the API restarted.

`TestOutagesFailFast` (`backend/test/integration/health_test.go`) keeps both fixes in place.

## Backup and restore

### Restore drill

`make restore-drill` (`scripts/restore-drill.sh`) runs against the idle stack:

1. Seeds a tenant with 25 events.
2. Backs up the roles (`pg_dumpall --roles-only`) and the database (`pg_dump -Fc`).
3. Restores both into a fresh PostgreSQL container that shares nothing with the live one.
4. Proves the restore:
   - every table's row count matches the live database;
   - every tenant's audit hash chain verifies (`riskosctl audit verify`, as the owner);
   - row-level security still isolates tenants for `riskos_app`;
   - an API instance on the restored database serves the seeded events with the seeded API key.

Results on 2026-10-01, same laptop:
- the database was 1.5 GB on disk (288,057 events, 12 tenants);
- the backup was 108 MB compressed.

| Step | Time |
|---|---|
| Backup | 43 s |
| Restore (including index builds) | 67 s |
| Verification | 60 s |
| Checks | 38 tables match, 12 audit chains verify, isolation holds, the API serves the data |

Restore time grows with data size. Rerun the drill and update this table when the data set
changes materially.

### Restore procedure

1. Declare an incident ([INCIDENT_RESPONSE.md](INCIDENT_RESPONSE.md)). Stop the API and the
   workers, so nothing writes to a database that is about to be replaced.
2. Restore into a new server, never over the damaged one: roles first, then the database
   (`pg_restore --exit-on-error`). In production this is the managed database's point-in-time
   recovery to just before the incident.
3. Verify before switching over:
   - run `riskosctl audit verify --tenant …` for every tenant;
   - compare row counts with the last known figures;
   - check that `migrate status` shows the expected version.
4. Point `RISKOS_POSTGRES_URL` and `RISKOS_POSTGRES_MIGRATION_URL` at the new server. Start the
   API, then the workers. Jobs in the restored queue resume.
5. Tell affected customers which window of data was lost, if any. Events in that window can be
   resent: event IDs make resending safe.

### Losing Redis state

Redis holds behavioural state, not records. Its append-only file survives restarts: the drill's
Redis stop loses nothing.

If the state itself is lost:
- rate limits reset;
- velocity windows restart;
- every user's history ("new device", "new beneficiary", amount baselines) starts again.

Changing `RISKOS_FEATURE_HASH_KEY` has the same effect: every stored history becomes unreachable.

**Detection.** An epoch marker is kept in Redis (`rk:epoch`, never expires) and in PostgreSQL
(`feature_store_state`), with a fingerprint of the hash key (a keyed hash, never the key). Each
API instance compares them at start-up and every 30 seconds, in the background. A missing marker
or a different fingerprint is recorded as a reset (`reset_at`), and an error is logged:
`feature store state was reset; history signals are warming up`.

**Warm-up.** For `RISKOS_HISTORY_WARMUP` after a reset (default 7 days, an assumption to tune
against how often customers' users return), the history signals that would mislead are held
back: `new_user`, `new_device`, `new_beneficiary`, `geo_anomaly` and `amount_anomaly`. Without the
history, every returning user's device, beneficiary and country would look new. The decision
response lists them in `warming_signals`, so customers can see that coverage is lower. Warming is
not degradation: fail policies do not apply, because a reset is not something a caller can cause
event by event. Velocity, network and credential-change signals keep working: they need only
recent observations.

Restoring PostgreSQL from a backup taken before the last reset also counts as a reset (the epochs
differ), which errs towards holding signals back.

Tested by flushing Redis and changing the recorded fingerprint (`TestFeatureStoreReset`).

## Not built yet

- Automated, encrypted, immutable backups with point-in-time recovery, held in a separate
  account. These come with the managed database. TODO(phase-8)
- Monthly restore tests against those backups. The drill above proves the procedure and the
  checks, not a production backup pipeline. TODO(phase-8)
- A zone and region failover design and its test. ARCHITECTURE.md §14 places a primary and
  replicas in the data zone; failover between zones or regions is not designed yet.
  TODO(phase-8)
