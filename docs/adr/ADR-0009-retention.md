# ADR-0009: Retention of the data RiskOS holds

- **Status:** accepted
- **Date:** 2026-10-10
- **Deciders:** product owner (delegated the choice to engineering's recommendation), engineering

## Context

ARCHITECTURE.md §6 specifies a retention sweep that drops expired partitions, with hot retention
of 90 days "configurable per tenant", and lists the sweep as not built. Three facts, two of them
learned only by reading the rule packs, change what a sweep may safely do:

1. **Legal floors are long.** The record-retention rules require at least five years in Nigeria
   (GOV-08) and Ghana (Act 1044 s.32), six years for payment-platform audit logs in Ghana
   (GH-SEC-11), and ten years after the relationship ends in Rwanda (RW-GOV-10, with the 2024
   reading at five). They apply to every record type the retention rules name, `other` included,
   and the longest duty wins over a data-protection request or a disposal policy.
2. **The floor is already computed per tenant.** `GET /api/v1/compliance/retention` and
   `/retention/check` return the retention in force per record type across the countries a tenant
   has activated, counting rules switched off, and audit every answer
   ([compliance/SECURITY_REVIEW.md](../compliance/SECURITY_REVIEW.md)). Every new workspace gets
   an automatic Nigeria activation, so every tenant has a five-year floor from its first event.
3. **Partitions are by time, not tenant.** A monthly partition holds events of every tenant, so
   it may be dropped only once every tenant in it is past its own floor: in practice, the
   longest floor in force on the platform.

The consequence is that nothing RiskOS stores becomes deletable before October 2031, and a
90-day hot tier is only valid if what leaves it goes somewhere (an archive tier), which depends on
the undecided deployment platform ([DEPLOYMENT.md](../DEPLOYMENT.md#platform-decision)).

## Decision

1. **The floor always wins.** No process deletes or drops a record before the retention in force
   for its record type and tenant. A tenant-configured period can lengthen retention, never
   shorten it below the floor. The deciding computation is the existing retention check, not a
   second implementation.
2. **Record types.** Events, evaluations and idempotency records are `transaction`; alerts, cases
   and webhook deliveries are `other`; the audit log is `audit_log`. A record's retention starts
   at its creation (an event's `received_at`), except customer files, which RiskOS does not hold.
3. **No destructive sweep is built yet.** Code that cannot be exercised for five years, that
   destroys data when wrong, and that needs a platform-dependent archive tier first is a liability.
   The sweep is built when the first of these is true, and not before:
   - the oldest partition is within 12 months of the shortest floor in force on the platform;
   - storage cost makes an archive tier worthwhile earlier, in which case the archive (export,
     verification, restore test) comes first and deletion follows it;
   - a regulator or a signed customer contract requires a shorter period for data that no floor
     covers.
4. **When it is built** it will: run as a scheduled job; compute eligibility per partition from
   every tenant's floor; drop only whole partitions; refuse unless an archive copy is verified
   (when an archive is in use); audit every run with counts and the winning rules, never
   content; need a second person's approval for any change to retention configuration
   (ARCHITECTURE.md §16); and have a dry-run mode that is the default.
5. **Documents say so.** ARCHITECTURE.md's retention row and EVENTS.md and WEBHOOKS.md describe
   the sweep as deferred by this decision, not as an oversight, and the "90 days hot" figure is
   a storage-tier target that applies only with an archive.

## Alternatives considered

- **Build the sweep now with 13-month defaults.** Rejected: it would delete records the rule
  packs say must be kept for five to ten years, for every tenant, from the day it ran.
- **Build it now with the floors enforced.** Safe, but it would delete nothing until 2031, so it
  is untestable against real data and adds a destructive code path with no present benefit.
- **Per-tenant row deletion instead of partition drops.** Needed only if one tenant could be
  deleted before others (an offboarded tenant, a data-subject request after the floor). Out of
  scope until a tenant offboarding process exists; the floor check applies the same way.
- **Drop only raw payloads early (keep normalized data).** A storage lever in ARCHITECTURE.md §6,
  but the raw payload is part of the record the floor covers, and the events table is insert-only
  by design; treated as part of the same deferred sweep.

## Consequences

- Storage grows for at least five years. The capacity plan in ARCHITECTURE.md §6 (about 2.3 TB
  hot) must be read with that in mind: the platform decision needs a cold or archive tier.
- Data-subject deletion requests are answered by the existing check, which refuses while a
  floor applies; there is nothing for RiskOS to delete before then.
- Offboarding a tenant is a separate decision (export, then deletion after the floor) that this
  ADR does not take.
- Follow-ups when the platform is chosen: an archive tier and the partition job per decision 4;
  a `retention_eligible_on` figure on the console's retention page.
