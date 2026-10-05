# Rule pack tracker

Open items of the rule pack work ([ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md),
[ADR-0007](../adr/ADR-0007-multi-jurisdiction.md)), each with the phase that closes it. An item
leaves this list in the commit that resolves it.

## Ghana and Rwanda (ADR-0007)

| Item | Phase |
|---|---|
| Benchmarks of three packs (including `days_over`, which reads its whole window, at most 20,000 entries, on a busy wallet), security review of the new endpoints, accessibility pass | 6 |

Nigeria: none open; the last items closed in phase 6. Follow-ups that need deployment infrastructure are
listed with their `TODO(phase-8)` in [SECURITY_REVIEW.md](SECURITY_REVIEW.md#accepted-risks),
[ACCESSIBILITY.md](ACCESSIBILITY.md#not-covered) and
[PERFORMANCE_BASELINE.md](../PERFORMANCE_BASELINE.md) §6.

## Decisions confirmed (Phase 3 review)

- The optional `compliance` block on every event and the `activity` event type are the public
  contract for rule pack data (EVENTS.md#compliance).
- The six new decisions are a breaking change for clients that switch exhaustively on the
  decision, and `DecisionCounts` lists all ten.
- An exemption that cannot be checked never lifts a legal limit (`exemption_unknown`).
- Changing a window filter's setting starts a fresh window.
