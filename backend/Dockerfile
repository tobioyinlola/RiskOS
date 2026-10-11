# syntax=docker/dockerfile:1.7
#
# Multi-stage build for the RiskOS backend binaries (api, worker, migrate).
# Runtime images are distroless/static running as nonroot: no shell, no package manager.
# Base images are pinned by digest; Dependabot keeps them current (.github/dependabot.yml).

ARG GO_IMAGE=golang:1.27.2-trixie@sha256:e58d6f83b3416618d8bcac2b3dde1b7f7e3c4a77d25e88637f8bbae81536c48d
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

# ---- source + module cache ---------------------------------------------------------------
FROM ${GO_IMAGE} AS source
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .

# ---- build --------------------------------------------------------------------------------
FROM source AS build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    for cmd in api worker migrate riskosctl; do \
      go build -ldflags="-s -w" -o /out/$cmd ./cmd/$cmd || exit 1; \
    done

# ---- development: hot reload with air (docker compose --profile dev) -----------------------
FROM ${GO_IMAGE} AS dev
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
RUN --mount=type=cache,target=/go/pkg/mod go install github.com/air-verse/air@v1.67.4
# Source is bind-mounted at /src; CMD is set per service in docker-compose.yml.

# ---- runtime images -----------------------------------------------------------------------
FROM ${RUNTIME_IMAGE} AS api
COPY --from=build /out/api /app/api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM ${RUNTIME_IMAGE} AS worker
COPY --from=build /out/worker /app/worker
USER nonroot:nonroot
ENTRYPOINT ["/app/worker"]

FROM ${RUNTIME_IMAGE} AS migrate
COPY --from=build /out/migrate /app/migrate
USER nonroot:nonroot
ENTRYPOINT ["/app/migrate"]
CMD ["up"]

FROM ${RUNTIME_IMAGE} AS riskosctl
COPY --from=build /out/riskosctl /app/riskosctl
USER nonroot:nonroot
ENTRYPOINT ["/app/riskosctl"]
