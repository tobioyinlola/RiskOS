# Deployment

**Purpose:** environments, build artifacts, configuration and the release process.

## Artifacts (in place)

Four images are built from this repository (`make docker-build`, CI job `docker`):

| Image | Dockerfile target | Runtime |
|---|---|---|
| api | `backend/Dockerfile` → `api` | distroless/static, nonroot |
| worker | `backend/Dockerfile` → `worker` | distroless/static, nonroot |
| migrate | `backend/Dockerfile` → `migrate` | distroless/static, nonroot |
| frontend | `frontend/Dockerfile` → `runtime` | distroless/nodejs24, nonroot |

Release order: run `migrate` to completion, then roll `api` and `worker`. The API fails readiness
first on SIGTERM and then drains, so rolling updates do not drop in-flight requests.

Configuration is environment-only (`RISKOS_*`; see `backend/internal/platform/config`).

## Platform decision

**Not made yet** (owner's call, 2026-10-10). Everything below is written as requirements any
platform must meet, so the choice can be made on cost, team skill and region availability
without reopening the design. When it is made, record it as an ADR and replace each
requirement's "provided by" with the concrete service. TODO(phase-8)

The code constrains the platform in only these ways: PostgreSQL with row-level security
enforced for the application role, Redis (single logical instance, loss tolerated), OTLP/HTTP
for telemetry, SMTP for mail, and outbound HTTPS from the worker. Nothing depends on a cloud
vendor API.

## Environments

| Environment | Purpose | Data | Access |
|---|---|---|---|
| local | development (`make up`) | synthetic | developers |
| staging | integration and release rehearsal; every `staging` commit | synthetic only, never copied from production | engineers |
| production | customers | real | named individuals, time-boxed, audited |

- Staging and production share no credentials, databases, queues, keys or networks. A secret
  that works in both is a defect.
- The product's own test and live environments (`rsk_test_` / `rsk_live_`) are tenant data
  within one deployment, separated by environment scope in every table
  ([TENANCY.md](TENANCY.md)); they are not separate deployments.
- Production access is by a short-lived grant with a recorded reason, not standing
  credentials. Database access for humans is read-only by default; a write session needs a
  second person's approval and is recorded.
- Changes to production come only from the release pipeline, never from a laptop.

## Network and edge

Requirements, in order of importance:

1. PostgreSQL and Redis are reachable only from the api, worker and migrate workloads: no
   public address, no human path except the audited access above.
2. Public traffic terminates TLS (1.2 or later) at an edge that also provides a WAF and a
   per-address rate limit. This closes the gaps noted in
   [ARCHITECTURE.md](ARCHITECTURE.md): customers' servers share addresses, so a per-address
   limit on all traffic belongs here, not in the application.
3. Traffic inside the platform is encrypted or on a network the platform isolates per
   workload. The database connection requires TLS.
4. The worker's egress is the only path from the platform to customer webhook URLs. It goes
   through an egress proxy or firewall policy that denies private, link-local and metadata
   ranges, in addition to the dialer's resolved-address check
   ([WEBHOOKS.md](WEBHOOKS.md)). Defence in depth: either alone is not the plan.
5. `RISKOS_TRUSTED_PROXIES` is set to exactly the edge's address range so client addresses are
   not spoofable.
6. The console and API are served from one registrable domain family chosen once, since console
   cookies, WebAuthn relying party ID and origins depend on it and are awkward to change.

## Secrets

- Held in a secrets manager and injected as environment variables at start; never in images,
  repository, CI logs or infrastructure-as-code state in clear text.
- Distinct per environment: database credentials (the application role is not the schema
  owner), the API key pepper, console session secret, webhook signing master key, SMTP
  credentials, identity provider client secrets.
- The API key pepper and the webhook signing key cannot be rotated by overwriting: the pepper
  invalidates every stored key hash, and the signing key changes every endpoint secret. Each
  needs a planned rotation procedure with an overlap period before first production use
  (webhook secrets already have a per-endpoint overlap; the master key does not).
  TODO(phase-8)
- Secret access is logged, and a secret read by a human is treated as an incident-level event.

## Supply chain and release controls

- Images are built only by the pipeline, from a commit on `main` that passed every required CI
  check; they are tagged by commit digest, never by a moving tag, and deployed by digest.
- Each image is signed and has build provenance and an SBOM attached; admission or deploy
  verification refuses unsigned images. (CI already produces SBOMs and runs gosec, CodeQL,
  govulncheck, Trivy and gitleaks; signing and provenance need the registry.)
- A production deployment needs approval by someone other than the author of the change, and
  is recorded (what, who, when, which digest).
- Rollback is a redeploy of the previous digest and must stay possible: see the migration rules.
- Releases happen in business hours unless they are fixes for an incident.

## Migration safety (expand and contract)

Migrations run before the new api and worker roll out, and the old version keeps serving during
the roll, so every migration must be compatible with both the version before and the version
after. In practice:

1. **Expand:** add nullable columns, new tables, new indexes (`CREATE INDEX CONCURRENTLY`), and
   backfill in batches. Never rename or drop in this step.
2. **Migrate:** deploy code that writes both and reads the new; backfill finishes.
3. **Contract:** in a later release, once no running version reads the old shape, drop it.
4. Never in one release: rename a column or table, change a type in place, add a `NOT NULL`
   without a default to a populated table, or drop anything the previous release reads.
5. A migration that takes a lock for longer than a second on a busy table is a defect; set
   `lock_timeout` so it fails instead of queueing behind traffic.
6. Each migration states how it is undone, or that it is not reversible and why. A release
   that contains an irreversible migration is not rolled back by redeploying; its rollback
   plan is written in the pull request.
7. Time-partitioned tables are created ahead by the worker's daily job
   (`EnsurePartitions`); a deployment must not depend on that job having run.

## Not built yet

- The platform, its infrastructure as code, the pipeline's deploy and approval stages, image
  signing and the secrets manager integration, all following the platform decision.
  TODO(phase-8)
- Master key rotation procedures (above).
