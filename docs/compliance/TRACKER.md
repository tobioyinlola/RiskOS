# Rule pack tracker

Open items of the rule pack work ([ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md),
[ADR-0007](../adr/ADR-0007-multi-jurisdiction.md)), each with the phase that closes it. An item
leaves this list in the commit that resolves it.

## Ghana and Rwanda (ADR-0007)

| Item | Phase |
|---|---|
| Calendar windows in the jurisdiction's time zone with a day boundary (GH-REG-01 and GH-REG-03 count the last 24 hours and 30 days until then; GH-TM-07's three-days-a-week test) | 3 |
| `txn.from_new_sender` (GH-TM-23) derived from history | 3 |
| Table-driven firing, non-firing and boundary tests for every numeric rule of both packs | 3 |
| Activation wizard: readiness check, maker-checker for live, rationale, change history; a setting to drop or change the fallback; `jurisdiction` on the configuration endpoints (inherited IDs resolve to the fallback today), so another country's rules are configured on their own pages | 4 |
| Marking an open question verified against the original text (who, when, a link or note), updating the badge on every rule it names | 4 |
| Activated countries' rules listed under Your rules next to the institution's own | 4 |
| Enabling a rule that is off until confirmed requires its placeholder settings and a rationale | 4 |
| Working-day clocks (GH-REG-11 holds 11 calendar days by default) | 5 |
| Data-subject clocks: RW-SEC-04 for a Rwandan data subject on a Ghana-booked event (`data_subject_country`) | 5 |
| Live counts of items running against each clock in the deadlines matrix; the cross-border view (events that touched more than one jurisdiction and the clocks they started) | 5 |
| Tasks carry their jurisdiction; reports for the FIC (Ghana, Rwanda) drafted without the NFIU's goAML check (REG-19 applies to Nigeria) | 5 |
| Retention across jurisdictions (longest period wins, RW-GOV-10's logged denial) | 5 |
| No-tipping-off serializer walk over gh and rw payloads | 5 |
| Benchmarks of three packs, security review of the new endpoints, accessibility pass | 6 |

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
