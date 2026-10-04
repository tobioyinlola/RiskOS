# Contributing

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) first. It explains the module boundaries that
the compiler and linters enforce.

## Workflow

Branches:

- **`main`** is the reviewed, releasable line. It changes **only** through a pull request from
  `staging` on GitHub. The pre-push hook rejects direct pushes to `main`.
- **`staging`** is where work lands. Pushing to it starts no CI workflow: the pre-push hook runs
  `make verify` instead, and CI and CodeQL run on pull requests to `main`. Dependabot also targets `staging`.
- Larger changes may use short-lived branches (`feat/<topic>`, `fix/<topic>`) merged into
  `staging` first.

Steps:

1. Work on `staging` (or a topic branch cut from it).
2. API change? Edit `backend/api/openapi.yaml` **first**, run `make generate`, then implement.
3. Keep commits small and logical, using [Conventional Commits](https://www.conventionalcommits.org):
   `feat(events): validate event timestamps`. The `commit-msg` hook checks this.
4. Run `make check` (what CI runs) before pushing. At minimum, run `make lint test-race`.
5. `git push origin staging`. The pre-push hook runs `make verify` (generated code, both lints,
   secrets, unit tests with the race detector, frontend tests) and refuses the push if it fails.
6. On GitHub, open a pull request **staging → main** and complete the **security checklist** in
   the template.

Significant or hard-to-reverse decisions get an ADR in `docs/adr/` (copy `TEMPLATE.md`).

## Go conventions

- **Errors:** wrap with context using `fmt.Errorf("verb object: %w", err)`, lower-case and without
  trailing punctuation. Combine with `errors.Join`. Check with `errors.Is` and `errors.As`, never
  string matching. Never ignore an error; if discarding one is correct, write `_ =` with a reason.
- **Context first:** every function that does I/O or may block takes `ctx context.Context` as its
  first parameter and honors cancellation. Never store a context in a struct.
- **No globals:** no package-level mutable state and no `init()` side effects. Dependencies are
  built in `cmd/*/main.go` and passed explicitly. Configuration is read only there.
- **Boundaries:** handlers call services, services call stores, only stores touch the database.
  Other modules are reached only through their public root package.
- **No panics for control flow.** Return errors. Panics are for programmer errors only.
- **Logging:** use the injected `*slog.Logger` with `…Context(ctx, …)` so request IDs attach.
  Never log credentials, request bodies or personal data.
- **Time:** inject `clock.Clock` where behavior depends on the current time.
- **Tests:** table-driven with `t.Run`; `testify/require` for preconditions and `assert` for
  checks. Integration tests use real PostgreSQL/Redis via testcontainers (`//go:build
  integration`), never database mocks. Security-relevant behavior gets a test in `test/security`.
- **Dependencies:** prefer the standard library. A new dependency needs a one-line reason in the
  PR and must pass `make vuln`.

## Database migrations

- Add a new numbered file in `backend/db/migrations/` (`goose create -dir db/migrations name sql`
  via `go tool -modfile=tools/go.mod goose`). Never edit a migration that has been merged.
- Every migration has a working `Down` or states why it cannot.
- Prefer additive (expand/contract) changes. Avoid long locks on large tables, and create indexes
  `CONCURRENTLY` (with `-- +goose NO TRANSACTION`).
- Queries go in `backend/db/queries/<module>/` and are generated with `make generate`.

## Frontend conventions

- TypeScript strict; no `any`. API types come only from the generated client (`@/lib/api/client`).
- Server components by default; add `"use client"` only for interactivity.
- Never render fabricated data. Show real API data or an explicit empty or not-built state.
- Only `NEXT_PUBLIC_*` variables may reach the browser, and they must never hold secrets.

## TODO markers

Bare `TODO`, `FIXME`, `XXX` and `HACK` comments are rejected by the linters (godox for Go,
`no-warning-comments` for the frontend). Write `TODO(phase-N): what and why`, naming the
implementation phase (spec §19) that resolves it.
