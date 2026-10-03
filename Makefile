# RiskOS developer entry point. `make help` lists targets.
# `make check` runs exactly what CI runs; keep the two in sync.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

BIN      := $(CURDIR)/bin
GO_TOOL  := go tool -modfile=tools/go.mod
COMPOSE  := docker compose
FRONTEND := cd frontend &&
BACKEND  := cd backend &&

export PATH := $(BIN):$(PATH)
export COREPACK_ENABLE_DOWNLOAD_PROMPT := 0
export NEXT_TELEMETRY_DISABLED := 1

##@ Environment

.PHONY: up
up: .env ## Build and start all services (production-like images) and wait until healthy
	$(COMPOSE) --profile app up --build --detach --wait --wait-timeout 180

.PHONY: dev
dev: .env ## Start with hot reload (backend: air, frontend: next dev)
	$(COMPOSE) --profile dev up --build

.PHONY: down
down: ## Stop all services (data volumes are kept; use `make clean` to remove them)
	$(COMPOSE) --profile app --profile dev down --remove-orphans

.PHONY: clean
clean: ## Stop all services and delete local data volumes
	$(COMPOSE) --profile app --profile dev --profile tools down --remove-orphans --volumes

.PHONY: logs
logs: ## Follow logs of all services
	$(COMPOSE) --profile app --profile dev logs --follow --tail=100

.PHONY: ps
ps: ## Show service status
	$(COMPOSE) --profile app --profile dev ps

.PHONY: migrate
migrate: .env ## Apply database migrations
	$(COMPOSE) --profile app run --rm --build migrate up

##@ Operations (riskosctl; the stack must be running)

RISKOSCTL := $(COMPOSE) --profile tools run --rm --build riskosctl

.PHONY: tenant
tenant: .env ## Create a tenant: make tenant NAME="Acme" REGION=eu-west-1 OPERATOR=you@example.com
	$(RISKOSCTL) tenant create --operator "$(OPERATOR)" --name "$(NAME)" --region "$(REGION)"

.PHONY: apikey
apikey: .env ## Issue a key: make apikey TENANT=ten_... ENV=production NAME=ci OPERATOR=you@example.com
	$(RISKOSCTL) apikey create --operator "$(OPERATOR)" --tenant "$(TENANT)" --env "$(ENV)" --name "$(NAME)"

.PHONY: invite
invite: .env ## Invite a console user: make invite TENANT=ten_... EMAIL=ada@example.com OPERATOR=you@example.com [ROLE=customer_admin]
	$(RISKOSCTL) user invite --operator "$(OPERATOR)" --tenant "$(TENANT)" --email "$(EMAIL)" --role "$(or $(ROLE),customer_admin)"

.PHONY: staff
staff: .env ## Create a RiskOS staff account: make staff EMAIL=ops@riskos.example NAME="Ops" OPERATOR=you@example.com [ROLE=operations_admin]
	$(RISKOSCTL) staff create --operator "$(OPERATOR)" --email "$(EMAIL)" --name "$(NAME)" --role "$(or $(ROLE),operations_admin)"

.PHONY: audit-verify
audit-verify: .env ## Verify a tenant's audit hash chain: make audit-verify TENANT=ten_...
	$(RISKOSCTL) audit verify --tenant "$(TENANT)"

.env:
	@echo "No .env found. Run: cp .env.example .env" >&2; exit 1

##@ Code generation

.PHONY: generate
generate: ## Regenerate server stubs, SQL code and the TypeScript API client
	$(BACKEND) $(GO_TOOL) oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	$(BACKEND) $(GO_TOOL) oapi-codegen -config api/oapi-codegen-admin.yaml api/admin.yaml
	$(BACKEND) $(GO_TOOL) sqlc generate
	$(FRONTEND) pnpm --silent generate:api

GENERATED := backend/internal/apigen backend/internal/adminapi backend/internal/platform/db/sqlcgen frontend/lib/api/schema.gen.ts frontend/lib/api/admin.gen.ts

.PHONY: generate-check
generate-check: generate ## Fail if generated code is not up to date with its sources
	@if ! git diff --quiet -- $(GENERATED) || [ -n "$$(git ls-files --others --exclude-standard -- $(GENERATED))" ]; then \
	  echo "Generated code is out of date. Run 'make generate' and commit the result:" >&2; \
	  git status --short -- $(GENERATED) >&2; git diff --stat -- $(GENERATED) >&2; exit 1; \
	fi

##@ Quality

.PHONY: fmt
fmt: ## Format Go and frontend code
	$(BACKEND) golangci-lint fmt ./...
	$(FRONTEND) pnpm --silent format

.PHONY: lint
lint: lint-backend lint-frontend ## Lint backend and frontend
	./scripts/check-todos.sh

.PHONY: lint-backend
lint-backend:
	$(BACKEND) golangci-lint run ./...

.PHONY: lint-frontend
lint-frontend:
	$(FRONTEND) pnpm --silent lint
	$(FRONTEND) pnpm --silent typecheck
	$(FRONTEND) pnpm --silent format:check

.PHONY: vuln
vuln: ## Scan dependencies for known vulnerabilities
	$(BACKEND) $(GO_TOOL) govulncheck ./...
	$(FRONTEND) pnpm audit --prod --audit-level=moderate

.PHONY: secrets
secrets: ## Scan the working tree and git history for committed secrets
	gitleaks git --redact --no-banner .
	gitleaks dir --redact --no-banner .

##@ Tests

.PHONY: test
test: test-backend test-frontend ## Unit tests (backend and frontend)

.PHONY: test-backend
test-backend:
	$(BACKEND) go test ./...

.PHONY: test-race
test-race: ## Backend unit tests with the race detector and coverage
	$(BACKEND) go test -race -covermode=atomic -coverprofile=coverage.out ./...
	$(BACKEND) go tool cover -func=coverage.out | tail -1

.PHONY: test-integration
test-integration: ## Backend integration and security tests against real Postgres/Redis (Docker)
	$(BACKEND) go test -race -count=1 -tags=integration ./test/...

.PHONY: test-frontend
test-frontend:
	$(FRONTEND) pnpm --silent test

.PHONY: test-e2e
test-e2e: ## Frontend end-to-end tests against the production build
	$(FRONTEND) pnpm --silent build
	$(FRONTEND) pnpm --silent test:e2e

.PHONY: test-e2e-stack
test-e2e-stack: .env ## Browser test against the running stack (make up): invite, passkey, every section
	./scripts/e2e-stack.sh

.PHONY: bench
bench: ## Backend micro-benchmarks
	$(BACKEND) go test -run='^$$' -bench=. -benchmem -count=5 ./... | tee bench.txt

.PHONY: load
load: ## Load test the running API (make up first; RATE=req/s DURATION=30s); results in backend/test/load/results
	mkdir -p backend/test/load/results
	docker run --rm --network host --user "$$(id -u):$$(id -g)" -v "$(CURDIR)/backend/test/load:/scripts" \
	  -e API_URL=$${API_URL:-http://127.0.0.1:8080} -e RATE -e DURATION \
	  grafana/k6:2.3.0@sha256:9c2dee7f8ed74d317e4027c06a10f169b625638189de8d4555d0b3486a5aeb34 \
	  run --summary-export=/scripts/results/health-summary.json /scripts/health.js

.PHONY: load-events
load-events: ## Load test event ingest (make up first): make load-events API_KEY=rsk_test_... [RATE=500] [PACK=1]
	@test -n "$(API_KEY)" || { echo "API_KEY is required (a key with events:write)" >&2; exit 1; }
	mkdir -p backend/test/load/results
	docker run --rm --network host --user "$$(id -u):$$(id -g)" -v "$(CURDIR)/backend/test/load:/scripts" \
	  -e API_URL=$${API_URL:-http://127.0.0.1:8080} -e API_KEY -e RATE -e DURATION -e PACK -e ACCOUNTS \
	  grafana/k6:2.3.0@sha256:9c2dee7f8ed74d317e4027c06a10f169b625638189de8d4555d0b3486a5aeb34 \
	  run --summary-export=/scripts/results/events-summary.json /scripts/events.js

.PHONY: failure-drill
failure-drill: .env ## Ingest under load while Redis, Postgres, worker and API fail; checks nothing is lost (make up first)
	./scripts/failure-drill.sh

.PHONY: restore-drill
restore-drill: .env ## Back up, restore into a fresh server and prove the copy (make up first, stack idle)
	./scripts/restore-drill.sh

##@ CI

.PHONY: check
check: generate-check lint vuln secrets test-race test-integration test-frontend test-e2e docker-build ## Everything CI runs

.PHONY: docker-build
docker-build: ## Build all runtime images
	docker build --target api -t riskos/api:local backend
	docker build --target worker -t riskos/worker:local backend
	docker build --target migrate -t riskos/migrate:local backend
	docker build --target riskosctl -t riskos/riskosctl:local backend
	docker build --target runtime -t riskos/frontend:local frontend

##@ Setup

.PHONY: tools
tools: ## Install pinned dev tools into ./bin and frontend dependencies
	./scripts/install-tools.sh
	$(BACKEND) cd tools && go mod download
	$(FRONTEND) pnpm install --frozen-lockfile

.PHONY: hooks
hooks: ## Install git hooks (lefthook)
	lefthook install

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
