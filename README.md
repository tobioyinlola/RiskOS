# RiskOS

API-first financial risk intelligence infrastructure. A customer platform sends financial and
behavioral events; RiskOS authenticates, validates, normalizes and enriches them, evaluates rules,
velocity and behavioral signals, and synchronously returns an explainable risk score, level,
signals and decision (ALLOW / REVIEW / STEP-UP / BLOCK). High-risk activity raises alerts that
analysts investigate as cases, and every material action is audited.

Product specification (source of truth): [`docs/spec/`](docs/spec/RiskOS_Complete_Implementation_Specification.md).
Architecture: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

> **Status:** foundation only. The platform (HTTP stack, configuration, logging, telemetry,
> database, Redis, job queue, CI and quality gates) is in place. The domain (tenants, API keys,
> events, risk engine, alerts, cases, …) is designed next, in phase-0.

## Prerequisites

Linux or macOS, or **WSL2 on Windows** (clone inside the WSL filesystem, not `/mnt/c`).

| Tool | Version | Used for |
|---|---|---|
| Docker (Desktop or Engine) with Compose v2 | recent | running the stack, integration tests |
| Go | 1.27.x (pinned in `backend/go.mod`) | backend |
| Node.js | 24 LTS | frontend |
| pnpm | via `corepack enable pnpm` (pinned in `frontend/package.json`) | frontend |
| make, git, curl | any recent | tooling |

Only Docker, make and git are needed to **run** the stack; Go and Node are needed to develop.

## First run

```bash
git clone https://github.com/tobioyinlola/RiskOS.git riskos && cd riskos
cp .env.example .env
make up
curl -s localhost:8080/health/ready   # {"status":"ok","checks":{"postgres":"ok","redis":"ok"}}
open http://localhost:3000             # console shell
```

`make up` builds the images, starts PostgreSQL, Redis and the OpenTelemetry Collector, runs
migrations, then starts the API, worker and frontend, and waits until everything is healthy.
`make down` stops the stack (keeping data); `make clean` also deletes the volumes.

## Everyday workflows

```bash
make tools        # once: pinned golangci-lint, gitleaks, lefthook into ./bin; frontend deps
make hooks        # once: install git hooks (format, lint, secrets, commit message)

make dev          # hot reload: backend via air, frontend via next dev
make logs         # follow logs
make migrate      # apply migrations

make generate     # after editing backend/api/openapi.yaml or db/queries: regenerate Go + TS code
make fmt          # format everything
make lint         # golangci-lint (incl. architecture rules), eslint, tsc, prettier
make test         # backend + frontend unit tests
make test-race    # backend tests with the race detector and coverage
make test-integration  # real PostgreSQL/Redis via testcontainers (needs Docker)
make test-e2e     # Playwright against the production frontend build
make vuln         # govulncheck + pnpm audit
make secrets      # gitleaks over history and working tree
make check        # everything CI runs
make help         # all targets
```

### Changing the API

1. Edit `backend/api/openapi.yaml`.
2. `make generate`: updates `backend/internal/apigen` and `frontend/lib/api/schema.gen.ts`.
3. Implement the new operation in the owning module's handler. The build fails until you do.
4. Commit the contract and the generated code together; CI rejects drift.

## Testing and benchmarking

| Layer | Location | Command |
|---|---|---|
| Backend unit | `backend/internal/**/*_test.go` | `make test-race` |
| Backend security (black-box HTTP) | `backend/test/security` | included in `make test` |
| Backend integration (real Postgres/Redis) | `backend/test/integration` | `make test-integration` |
| Frontend unit/component | `frontend/tests/unit` | `make test-frontend` |
| Frontend end-to-end | `frontend/tests/e2e` | `make test-e2e` |
| Micro-benchmarks | `Benchmark*` in backend | `make bench` |
| Load (k6) | `backend/test/load` | `make up && make load` |

Baseline numbers: [`docs/PERFORMANCE_BASELINE.md`](docs/PERFORMANCE_BASELINE.md).

## Repository map

```
backend/            Go: cmd/{api,worker,migrate}, internal/ (platform + domain modules),
                    api/openapi.yaml (contract), db/{migrations,queries}, test/
frontend/           Next.js console: app/, components/, lib/api (generated client), tests/
docs/               spec/, ARCHITECTURE.md, SECURITY.md, THREAT_MODEL.md, … , adr/
infra/              local infrastructure config (OpenTelemetry Collector)
scripts/            tool installation and repository checks
.github/            CI workflow, Dependabot, CODEOWNERS, PR template
docker-compose.yml  local stack (profiles: app, dev)
Makefile            single entry point for every workflow
```

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md). Report vulnerabilities privately as described in
[docs/SECURITY.md](docs/SECURITY.md).
