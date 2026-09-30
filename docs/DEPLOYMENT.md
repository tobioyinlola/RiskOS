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

## To be designed (TODO(phase-8))

- Target platform and networking: private database and Redis, WAF/API gateway, TLS everywhere.
- Separate test and production environments with controlled production access (spec §12).
- Secrets manager integration, image signing and provenance, deployment approvals.
- Migration safety rules for zero-downtime deploys (expand/contract).
