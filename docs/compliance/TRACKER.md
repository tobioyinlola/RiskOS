# Rule pack tracker

Open items of the Nigeria rule pack work ([ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md)),
each with the phase that closes it. An item leaves this list in the commit that resolves it.

| Item | Effect today | Closes in |
|---|---|---|
| `history.hour_usual` not tracked | BEH-03 not evaluated | Phase 4 (hour-of-day history per user) |
| `export.rows_to_user_baseline`, `export.rows_to_tenant_baseline` not tracked | SEC-12 not evaluated | Phase 4 (export size baselines; needs `export.rows` on data_export events) |
| `sender.related`, `flows.prior_sender_outflow_share` not tracked | PON-01 not evaluated | Phase 5 (money-flow history between accounts) |
| `geo.speed_from_previous` not tracked | BEH-02's impossible-travel branch not evaluated (the new-country branch works) | Phase 5 (needs event coordinates in the contract) |
| Workflow rules (REG-14 clocks, REG-19 report checks, governance presets) | Evaluated only for workflow events, which nothing emits yet | Phase 5 |
| GOV-09: compliance-only signals (INS-01) are labelled in API responses and the console | Visible to every role that sees the evaluation | Phase 5 (role-gated display, no tipping off) |
| End-to-end decision-path p95 with the pack, Redis included | Interpreter cost only is measured | Phase 6 |
| Live monitor e2e test occasionally fails under parallel load | Passes on rerun | Phase 6 |

## Decisions confirmed (Phase 3 review)

- The optional `compliance` block on every event and the `activity` event type are the public
  contract for rule pack data (EVENTS.md#compliance).
- The six new decisions are a breaking change for clients that switch exhaustively on the
  decision, and `DecisionCounts` lists all ten.
- An exemption that cannot be checked never lifts a legal limit (`exemption_unknown`).
- Changing a window filter's setting starts a fresh window.
