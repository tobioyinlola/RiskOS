# ADR-0007: Several jurisdictions per workspace; Ghana and Rwanda packs

- **Status:** accepted
- **Date:** 2026-10-04
- **Deciders:** product owner, engineering

## Context

ADR-0006 gave each workspace one compliance profile and one jurisdiction pack (Nigeria). Tenants
that operate in several countries need each transaction judged by the rules and legal clocks of
the country that governs it. The Ghana and Rwanda library
([GHANA_RWANDA_RULES_LIBRARY.md](../compliance/GHANA_RWANDA_RULES_LIBRARY.md)) defines 68 and 64
rules, reuses Nigerian rules (some parameterised, some unchanged), tags every rule with its
provenance and lineage, and leaves 15 rules off until a missing number is confirmed. Its evidence
is thinner than Nigeria's, and its conflicts (C1 to C20) must stay visible until a person
resolves them.

## Decision

1. **Jurisdictions are reference data.** A `jurisdictions` table holds each country's currency,
   minor unit, time zone, regulators and pack, seeded by migration and read-only to the
   application. A pack declares its currency and time zone; the loader refuses a pack whose
   country has no row or whose locale differs from it. A fourth country is a pack and a row.
2. **Activations replace the single profile.** The profile table becomes
   `jurisdiction_activations`: one row per jurisdiction and booking entity, with the licence
   type, capabilities, pack version and a status (draft, shadow, live, suspended). One activation
   may be the tenant's **fallback**. Existing profiles become the tenant's Nigeria activation for
   the booking entity `default`, flagged as the fallback, so Nigeria keeps working unchanged; new
   workspaces get the same automatic activation. The compliance profile endpoints read and write
   the fallback activation. Row-level security scopes every activation to its tenant.
3. **One governing pack per event, never guessed.** The event names its booking entity
   (`compliance.booking_entity`). Resolution: that entity's activation; else the stated fallback;
   else none (`unresolved`), and no pack judges the event. An entity in draft or suspended is
   `inactive`: no other country's pack stands in. The decision, the stored evaluation and its
   features record the country, pack version and how it was resolved. The governing pack's time
   zone sets hours of the day; its licence type is the `institution.licence_type` fact.
4. **Packs reuse parent rules as data, not copies.** A "Param of X" rule names X as its
   `template` and states only its overrides: the loader copies X's logic, and a test proves they
   share it. A pack's `inherits` list applies parent rules under their own IDs; every amount must
   be restated in the local currency (a naira default is never reused). A child pack's checksum
   includes its parents', so a changed parent forces a new child version. Rule configuration and
   pending changes are keyed by pack, so an inherited rule is configured per country.
5. **Country IDs and tags.** Country rules are `GH-REG-01`, `RW-TM-24`: REG controls, TM signals,
   GOV presets, and SEC either (some are legal clocks). Each carries its provenance (regulatory,
   industry_practice, my_design), lineage, off reason and the library's basis column verbatim;
   tests hold the seed to the library in both directions.
6. **Clocks may be unknown.** A clock is a legal clock, internal policy, a design placeholder, or
   not found; a clock not found has no hours and is never shown as zero. Controls can set a
   decision and tasks per branch (RURA's immediate versus monthly reports).

## Alternatives considered

- **Strictest rule of every country on every event.** Simple, but it applies laws to customers
  they do not govern and hides which law a decision rests on; rejected (library section 6).
- **Copying parent definitions into each country pack.** Easy to load, but the copies drift from
  their parent and nothing proves a "Param of X" rule is X.
- **No automatic fallback.** Purest, but it would stop evaluating every existing workspace's
  events until each named a booking entity. The fallback is kept, stated and visible.

## Consequences

- Clients that switch on clock hours must handle null and 0 (API changelog).
- Events without a booking entity keep the Nigeria behaviour until a tenant changes its fallback.
- Several rules read facts RiskOS derives from the event (`txn.direction`, `event.kind`,
  `txn.from_new_sender`). Calendar windows (a local day or month from the activation's
  `day_starts_at`) and `days_over` are computed the same way live (Redis) and in backtests; the
  tracker lists what each later phase completes.
