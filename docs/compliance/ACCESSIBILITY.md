# Rule pack accessibility

**Purpose:** the accessibility pass of the rule pack's console pages (Rules library, a rule, Work
queue), rule pack phase 6. Target: WCAG 2.1 AA, as for the rest of the console
([design-system.md](../design-system.md)). Reviewed 2026-10-03.

## Automated

`frontend/tests/e2e/a11y.spec.ts` runs axe-core (WCAG 2.1 A and AA rules) on the Rules library,
a rule, and the Work queue with every panel shown (tasks, both kinds of escalation, unreported
fraud, compliance-only signals) and a report dialog open, on desktop and mobile. It runs with the
rest of the end-to-end suite; a violation fails it, naming the rule and the elements. Automated
rules catch only part of what matters, so the checks below were made by hand.

## Fixed in this pass

| Issue | WCAG | Fix |
|---|---|---|
| Dialogs never announced their description: the shared `Modal` overrode Radix's `aria-describedby` even when it rendered one. "Required by REG-14 for customer …" and Confirm suspicion's "never tell the customer" were silent for screen-reader users | 1.3.1, 4.1.2 | `aria-describedby` is cleared only when a dialog has no description; the end-to-end test asserts the report dialog's description |
| Code blocks (a rule's definition and engineering logic, event payloads) scroll but could not be reached by keyboard | 2.1.1 | `CodeBlock` is a labelled, focusable region ("Rule definition", "Engineering logic") |
| A table wider than its card (mobile) scrolled sideways without keyboard access | 2.1.1 | The table's scroll container becomes a focusable region named after the table, only while it scrolls |
| Every task row's button was named "Open" | 2.4.6 | Named after the task: "Open Suspicious transaction report for usr_4812" |

## Checked by hand

- **Keyboard.** Every action on the three pages is a native button, link, select or input in
  reading order; dialogs trap focus, close on Escape and return focus to the button that opened
  them (Radix); the global `:focus-visible` ring shows on every focusable element.
- **Announcements.** Escalations are notices with an icon and words, never color alone: GOV-11's
  urgent ones are `role="alert"`, REG-14's `role="status"`. Deadlines are text ("due in 3h 20m",
  "overdue by 2d 4h"); overdue ones are also red, but the words carry it. Countdowns are as of
  the last refresh and do not tick, so screen readers are not interrupted.
- **Names.** Tables, filters and the library's search carry labels; "Prepare report" names its
  case; links to alerts and cases are their titles.
- **Motion.** Notice and dialog fades respect `prefers-reduced-motion` (`globals.css`).

## Not covered

- A screen-reader pass with NVDA or VoiceOver on real hardware (the checks above use the
  accessibility tree): schedule with the next console release (TODO(phase-8)).
- Pages outside the rule pack were not reviewed in this pass; the shared component fixes above
  apply to them too.
