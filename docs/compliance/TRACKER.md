# Rule pack tracker

Open items of the Nigeria rule pack work ([ADR-0006](../adr/ADR-0006-jurisdiction-rule-packs.md)),
each with the phase that closes it. An item leaves this list in the commit that resolves it.

| Item | Effect today | Closes in |
|---|---|---|
| End-to-end decision-path p95 with the pack, Redis included | Interpreter cost only is measured | Phase 6 |
| Live monitor e2e test occasionally fails under parallel load | Passes on rerun | Phase 6 |

## Decisions confirmed (Phase 3 review)

- The optional `compliance` block on every event and the `activity` event type are the public
  contract for rule pack data (EVENTS.md#compliance).
- The six new decisions are a breaking change for clients that switch exhaustively on the
  decision, and `DecisionCounts` lists all ten.
- An exemption that cannot be checked never lifts a legal limit (`exemption_unknown`).
- Changing a window filter's setting starts a fresh window.
