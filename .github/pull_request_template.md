## What and why

<!-- What does this change do, and why is it needed? Link the spec section or issue. -->

## How it was verified

<!-- Tests added/updated, commands run, screenshots for UI changes. -->

## Security checklist

Tick each item or explain why it does not apply.

- [ ] **Tenant scoping:** every new query and endpoint is scoped to the authenticated tenant; no tenant ID is taken from client input.
- [ ] **Authorization:** every new operation checks the caller's permissions server-side; negative tests cover denied access.
- [ ] **Input validation:** new inputs are validated against the OpenAPI contract and size limits.
- [ ] **Secrets and personal data:** nothing sensitive is logged, returned in errors, stored unhashed or placed in job arguments.
- [ ] **Contract:** `api/openapi.yaml` is updated first and `make generate` has been run; no undocumented endpoints.
- [ ] **Migrations:** reviewed for locking, backfill cost and rollback; no destructive change without a plan.
- [ ] **Audit:** material administrative and security actions are written to the audit log.
- [ ] **Synchronous path:** no slow or external work added to the request path of event ingestion or risk evaluation.
- [ ] **Dependencies:** any new dependency is justified in the description.

## Docs

- [ ] Relevant docs in `docs/` updated (architecture, security, threat model, ADR for significant decisions).
