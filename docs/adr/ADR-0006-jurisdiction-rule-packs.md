# ADR-0006: Jurisdiction rule packs with regulatory hard controls

- **Status:** accepted
- **Date:** 2026-10-02
- **Deciders:** product owner, engineering

## Context

Tenants regulated in Nigeria need the controls Nigerian law and regulators require (CBN, NFIU,
NIBSS, NCC, SEC, FCCPC) and a library of recommended typology rules, readable by compliance
officers and traceable to a legal source ([NIGERIA_RULES_LIBRARY.md](../compliance/NIGERIA_RULES_LIBRARY.md)).
Until now tenant rules could only add weight, BLOCK came only from automatic blocking after 14 days
in shadow mode, and hard blocks were limited to the tenant's own deny lists (ARCHITECTURE.md
decision 12). The regulatory controls must decline, hold or block regardless of the score.

## Decision

1. **Rule packs are data.** A jurisdiction pack (Nigeria first, `ng`) is a versioned set of
   declarative rule definitions embedded in the binary and loaded into versioned tables by the
   migrate step. Every threshold, window and weight is a named parameter with bounds; definitions
   carry their legal source, effective date, licence types and verification status. The engine
   interprets them; there is no `eval`, scripting or user-supplied code.
2. **Regulatory controls (REG) are hard controls and run live by default.** When one applies it
   sets the decision directly, regardless of the score. This is a deliberate exception to decision
   12 for regulatory controls only; recommended rules still only add weight. Per tenant, each
   control can be switched to shadow mode or off, and loosening or disabling a regulatory control
   needs a second approver and is audited. Controls whose legal basis is awaited ship off.
3. **The decision vocabulary grows.** `Decision` adds `decline`, `hold`, `block_session`,
   `restrict`, `flag` and `fee` next to `allow`, `review`, `step_up` and `block`. This is a
   breaking change for API clients that switch exhaustively on the decision, announced in the API
   changelog.
4. **Compliance roles.** Two customer roles are added: `compliance_officer` (the MLRO: reads the
   library, sees suspicion and STR data, approves reports) and `auditor` (read-only). Only
   compliance roles see STR existence, filing status and suspicion flags (GOV-09).
   Permissions: `compliance:read` (every customer role), `compliance:engineering` (section 7B
   logic and machine definitions; administrators and developers) and `compliance:write` (the
   compliance profile; people only, never API keys).
5. **Packs are global, versioned reference data.** `migrate up` validates each built-in pack and
   loads any version not yet stored into `rule_packs` and `pack_rules`, as the schema owner. The
   application role can only read them. A stored version is immutable: a trigger refuses updates
   and deletes, and loading the same version with a different checksum fails, so changed content
   needs a new version. Each tenant's `tenant_compliance` row (RLS) pins the pack version it is on
   and records its licence type and capabilities, which decide which rules apply.

## Alternatives considered

- **Controls in shadow per rule, live by tenant opt-in.** Safer, closer to decision 12; rejected
  by the product owner in favour of compliance by default for regulated tenants.
- **Advisory controls** (attach required actions, keep the decision from the score). Keeps
  decision 12 intact but leaves enforcement to every customer integration.
- **Keep four decisions and add a control-actions list.** Non-breaking; rejected in favour of a
  decision that states the required action directly.

## Consequences

- A tenant on a pack sees declines, holds and blocks that no score explains: each one names the
  control, its legal source and its evidence.
- Every control needs firing and non-firing fixtures and boundary tests before it runs live.
- API clients must handle the new decisions; the contract and changelog say so.
- Pack updates are new versions; tenants keep their overrides and see what changed.
