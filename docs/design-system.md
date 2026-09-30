# RiskOS Design System — "Instrument"

> Spec for Claude Code. Save this as `docs/design-system.md` and add this line to `CLAUDE.md`: "Follow `docs/design-system.md` for all UI work; never hard-code colours, spacing, radii or fonts." The stack is Next.js + TypeScript. Implement the tokens as CSS variables in `app/globals.css`, map them into Tailwind, and build components on an accessible headless layer (Radix or Base UI).

## Brand book

RiskOS is a precision instrument for money. Events go in and an explainable reading comes out, like a seismograph recording a tremor before the quake. Every design decision follows from that idea: graphite and paper grounds, one live signal colour, rulers and ticks instead of decoration, readouts instead of big numbers, and a trace line that runs through the whole brand.

### The idea: Instrument

- **Calibrated, not decorated.** Nothing on screen is ornamental. Every line is a measurement: a rule, a tick, a trace or a threshold.
- **One live signal.** Signal lime (`signal`) is the brand colour and means "this is live, this is ours". It is never a risk colour, never a gradient, and rarely covers more than 5% of a screen.
- **Readings, then reasons.** A score is shown as a reading on a scale (`RiskScore`), and the reasons come right after it (`ReasonList`). Never show a bare number.
- **Graphite for operators, paper for records.** The console runs on `dark` (Graphite). Docs, reports, exports and printed case files run on `light` (Paper), which is a warm off-white like chart paper, not pure white.

### Signature elements

These four things make RiskOS recognisable. Use them consistently, and don't invent others.

1. **The trace.** A 1.5px (`trace-stroke`) line that stays flat and then spikes. It crosses the symbol, underlines the active tab, sits in `TraceLine` sparklines, and draws the loading state (a flat line that pulses once). On dark, draw it in `signal`. On paper, draw it in `signal-ink`.
2. **The readout.** Headline scores use the `readout` style, always three digits, with leading zeros in `ink-3` (`087`), set in a `bg-sunken` well with square corners (`radius-0`). It looks like a meter, not a KPI.
3. **The ruler.** Scales are drawn as rulers: `tick-minor` every 2 points, `tick-major` with a `legend` number every 10, band thresholds in `line-control`. The needle is a 2px `ink` line.
4. **The band glyph.** The risk band is shown by the number of filled bars (1 to 5, like signal strength) as well as by colour and word. The band stays readable in greyscale, in print and for colour-blind analysts.

### Voice

- Write like an instrument's manual crossed with an analyst's case note: exact, calm, and with units. "Card used in 3 countries in 40 min", not "Suspicious activity detected!"
- Use sentence case, and "you" for the user. No "we", no exclamation marks, no emoji.
- Decisions are verbs (**Allow**, **Review**, **Step-up**, **Block**). Bands are adjectives (**Minimal**, **Low**, **Elevated**, **High**, **Critical**).
- Always give a number with its unit and window: "6× baseline (1h)", "38 ms", "4,500.00 NGN".
- Uppercase appears only in the mono `legend` style, like the printing on an instrument panel.
- Taglines are short and physical: "Read the tremor before the quake." "Every score, calibrated." "Risk, measured."

### Risk scale

| Band | Score | Bars | Default decision | Colour / wash |
|---|---|---|---|---|
| Minimal | 0–19 | ▮▯▯▯▯ | Allow | `risk-minimal` / `risk-minimal-wash` |
| Low | 20–49 | ▮▮▯▯▯ | Allow | `risk-low` / `risk-low-wash` |
| Elevated | 50–69 | ▮▮▮▯▯ | Review | `risk-elevated` / `risk-elevated-wash` |
| High | 70–89 | ▮▮▮▮▯ | Step-up or Block | `risk-high` / `risk-high-wash` |
| Critical | 90–100 | ▮▮▮▮▮ | Block + open case | `risk-critical` / `risk-critical-wash` |

Tenants can move the thresholds. When they do, read them from the policy API, redraw the ruler's threshold ticks, and show "custom calibration" in `legend` beside the scale.

> **Implementation note (RiskOS engine 2.0.0):** the bands and default thresholds above are the engine's. Until opt-in auto-block ships (build step 8), the engine's defaults are Minimal/Low Allow (Low steps up with a strengthenable signal), Elevated Review, High Step-up (Review where step-up is impossible) and Critical Review; the console never offers Block. Read the environment's actual decisions from `GET /api/v1/risk/policy` ([RISK_ENGINE.md](RISK_ENGINE.md)).

### Visual foundations

**Colour.** Grounds step `bg-canvas` → `bg-surface` → `bg-raised`, with `bg-sunken` for readout wells, code and header strips. Text is `ink`, `ink-2` and `ink-3`; all three pass 4.5:1 on every ground in both themes. Draw hairlines in `line-subtle` and control edges in `line-control` (3:1). Use `signal` only as a fill or as the trace. Use `signal-ink` for signal-coloured text and thin lines, because lime can't sit directly on paper. Primary buttons use `primary-fill` with an `on-primary` label: lime with graphite text in dark, graphite with lime text in light. That inversion is intentional and on-brand. `signal-wash` marks the selected row. There are no gradients, glows or glass effects.

**Type.** The interface is Instrument Sans: a tight, slightly condensed grotesk with instrument-like precision, set with negative tracking at large sizes. Every number, ID, legend and readout is JetBrains Mono with `font-variant-numeric: tabular-nums slashed-zero`. The two families split the work: sans is for people, mono is for the machine's readings. Load Instrument Sans 400/500/600 and JetBrains Mono 400/500 from Google Fonts.

**Grid and spacing.** Everything sits on a 4px grid (`space-1` … `space-8`). Covers, empty states and marketing pages may show grid paper: `line-subtle` lines at `space-4` minor and `space-8` major pitch. The product UI never does. The console layout is a left nav (232px), a filter bar, the queue table, and a 480px right drawer that opens over the table, so the analyst keeps their place.

**Shape.** Corners are nearly square: `radius-0` for readouts, rulers and tables, `radius-sm` for badges, `radius-md` for controls and `radius-lg` for panels. Only the live dot and the symbol's ring are round. Panels are flat with a `line-subtle` edge. Only floating layers take `shadow-overlay`.

**Focus and states.** Focus is a 2px solid `focus-ring` outline at 2px offset with square corners. Hover moves a row to `bg-raised`. Selected rows take `signal-wash` plus a 2px `signal-ink` bar at the row's left edge; this is the one place a left-edge bar is allowed, because it works as a needle. Disabled controls sit at 40% opacity. Loading shows a flat trace, or skeleton rows at the real row height.

**Motion.** Motion is mechanical and brief, 120–180ms with `cubic-bezier(0.2, 0, 0, 1)`. The needle eases to its value once. The trace draws left to right once on load. The live dot pulses at a slow 2s rhythm. New events slide in without moving rows the user is reading, and the stream pauses on hover. With `prefers-reduced-motion`, show all of these without animation.

**Charts.** Draw series as `trace-stroke` lines, not filled areas. The primary series uses `signal-ink` and comparisons or baselines use `ink-3` dashed. Axes are rulers with `legend` labels. When a chart encodes risk, it uses the `risk-*` tokens in band order. Band thresholds are `line-control` rules. Every chart has a table view.

### Logo

The symbol is a dial ring crossed by a trace spike: the instrument catching a reading. See the Logos asset group for the files and rules. The wordmark is the symbol followed by `riskos` in lowercase Instrument Sans 600 at -0.03em. Write "RiskOS" in running text.

### Iconography

Use Lucide at a 1.5px stroke (matching `trace-stroke`), square line caps, 16px inline and 20px in nav, inheriting the text colour. Required pairs: activity (trace) for live, gauge for score, shield for policy, fingerprint for device, map-pin for geo, timer for velocity, octagon for block. Don't use filled icons, duotone or emoji.

### Accessibility

Meet WCAG 2.2 AA as the floor. Every band is encoded three ways (bars, word, colour). Every queue action has a keyboard shortcut (`j`/`k` to move, `a` allow, `r` review, `b` block, `Enter` to open), shown in tooltips and in a `?` overlay. The readout exposes `role="meter"` with a spoken label, for example "Risk score 87 of 100, High".

> **Implementation note:** RiskOS queues triage alerts rather than deciding events, so the alert queue's shortcuts are `j`/`k` move, `Enter` open, `i` investigating, `f` false positive, `x` resolve, `c` open case, `?` help. Decisions (allow/review/block) are made by the engine, not by analysts, until manual decisions exist.

## Tokens

There are two themes: `dark` (Graphite, the default for the analyst console) and `light` (Paper, for docs, reports and exports). Set the theme with `data-theme` on `<html>`.

### Colour

| Token | Graphite (dark) | Paper (light) | Usage |
|---|---|---|---|
| `bg-canvas` | `#0e100f` | `#f3f1ea` | Page ground. Graphite in the console, warm paper in docs and reports. Carries the grid-paper texture on covers and empty states only. |
| `bg-surface` | `#151816` | `#fbfaf6` | Panels, tables, drawers. The default ground for every text token. |
| `bg-raised` | `#1d211f` | `#ffffff` | Menus, popovers, hovered row. |
| `bg-sunken` | `#090a0a` | `#e8e5da` | Readout wells behind scores, code and JSON, table header strip. |
| `line-subtle` | `#262b28` | `#dedad0` | Hairlines: row dividers, panel edges, minor ruler ticks, grid-paper lines. Decorative only. |
| `line-control` | `#68716a` | `#7c7a6c` | Input and secondary-button edges, major ruler ticks, band thresholds on charts. 3:1 on every ground. |
| `ink` | `#eceee6` | `#151816` | Primary text, scores, the needle, the symbol's ring. On every ground. |
| `ink-2` | `#a9afa3` | `#474d45` | Secondary text: labels, column headers, descriptions. On every ground. |
| `ink-3` | `#878e82` | `#5f655b` | Muted text: timestamps, IDs, units, the dimmed leading zeros of a readout. On every ground (4.5:1+). |
| `signal` | `#c8f03c` | `#c8f03c` | Signal lime, the brand. A FILL: primary button in dark, the live trace, the symbol's trace, the 'live' dot. On paper it only appears inside a graphite element, never directly on paper. |
| `signal-ink` | `#c8f03c` | `#4a6400` | Signal as TEXT or a thin line: links, active tab underline, selected nav marker, primary trace line on paper. On every ground. |
| `on-signal` | `#151816` | `#151816` | Text and icons on a signal fill. |
| `signal-wash` | `#1c2210` | `#eef3d6` | Selected row and active filter chip ground. Text on it stays ink. |
| `primary-fill` | `#c8f03c` | `#151816` | Primary button fill: lime on graphite, graphite on paper. |
| `on-primary` | `#151816` | `#c8f03c` | Primary button label: graphite on lime in dark, lime on graphite in light (13.6:1). |
| `focus-ring` | `#c8f03c` | `#4a6400` | 2px solid outline, 2px offset, square corners, on every focusable element. |
| `risk-minimal` | `#9aa39a` | `#5f655b` | Score 0-19, band glyph 1 bar. Text, needle zone and bar fill. |
| `risk-minimal-wash` | `#2a2e2b` | `#eaeae5` | Ground for a Minimal badge; text risk-minimal. |
| `risk-low` | `#7fb0ff` | `#2a5cc0` | Score 20-49, 2 bars. Blue: the scale never relies on red vs green. |
| `risk-low-wash` | `#26303b` | `#e4e9f0` | Ground for a Low badge; text risk-low. |
| `risk-elevated` | `#f2b33d` | `#8f5200` | Score 50-69, 3 bars. Default route: review. |
| `risk-elevated-wash` | `#38311c` | `#efe8db` | Ground for an Elevated badge; text risk-elevated. |
| `risk-high` | `#ff8547` | `#b03e0e` | Score 70-89, 4 bars. Default route: block or step-up. |
| `risk-high-wash` | `#3a291e` | `#f3e5dc` | Ground for a High badge; text risk-high. |
| `risk-critical` | `#ff5a73` | `#bb1437` | Score 90-100, 5 bars. Block and open a case. The only red in the product. |
| `risk-critical-wash` | `#3a2325` | `#f4e1e1` | Ground for a Critical badge; text risk-critical. |
| `status-ok` | alias of `signal-ink` | alias of `signal-ink` | System health (webhook delivered, key active, model healthy) is the signal colour: 'the line is live'. Always with the live dot or a check. |
| `status-warn` | alias of `risk-elevated` | alias of `risk-elevated` | System warnings (rate limit near, model drift). Always with a warning icon. |
| `status-error` | alias of `risk-critical` | alias of `risk-critical` | Destructive actions and system errors. Always with an error icon. |

### Typography

Load the fonts with `next/font/google`: Instrument Sans (400, 500, 600) and JetBrains Mono (400, 500). Apply `font-variant-numeric: tabular-nums slashed-zero` to every mono style.

- `sans`: `"Instrument Sans", ui-sans-serif, system-ui, sans-serif`
- `mono`: `"JetBrains Mono", ui-monospace, "SFMono-Regular", Menlo, monospace`

| Style | Family | Size / line height | Weight | Tracking | Usage |
|---|---|---|---|---|---|
| `display` | sans | 44px / 44px | 600 | -0.035em | Site, deck and empty-state headlines. Tight and large; one line where possible. |
| `title` | sans | 24px / 28px | 600 | -0.02em | Page titles. |
| `heading` | sans | 17px / 24px | 600 | -0.01em | Panel and drawer titles. |
| `body` | sans | 14px / 20px | 400 | 0 | Default UI text and table cells. |
| `body-sm` | sans | 13px / 18px | 400 | 0 | Compact tables, helper text. |
| `label` | sans | 12px / 16px | 500 | 0 | Form labels, badge words. |
| `readout` | mono | 56px / 56px | 500 | -0.04em | The headline score. Always three digits; leading zeros in ink-3. One per view. |
| `metric` | mono | 24px / 28px | 500 | -0.02em | KPI tile numbers. |
| `data` | mono | 13px / 18px | 400 | 0 | IDs, amounts, IPs, timestamps, keys, JSON. Tabular figures. |
| `legend` | mono | 11px / 14px | 500 | 0.08em | Uppercase instrument legends: column headers, KPI labels, ruler numbers, nav group names. |

### Spacing (4px grid)

| Token | Value | Usage |
|---|---|---|
| `space-1` | `4px` | Tick spacing on small rulers; icon-to-label gap. |
| `space-2` | `8px` | Gaps inside controls; badge padding. |
| `space-3` | `12px` | Table cell padding. |
| `space-4` | `16px` | Panel padding. Grid-paper minor pitch. |
| `space-5` | `24px` | Gap between panels; drawer padding. |
| `space-6` | `32px` | Page gutters. |
| `space-8` | `64px` | Grid-paper major pitch; empty states and page-top spacing. |

### Radius

| Token | Value | Usage |
|---|---|---|
| `radius-0` | `0px` | Readout wells, rulers, table, focus ring. Instruments have square faces. |
| `radius-sm` | `2px` | Badges, checkboxes, tags. |
| `radius-md` | `4px` | Buttons, inputs, menus. |
| `radius-lg` | `6px` | Panels, drawers, modals. |
| `radius-full` | `999px` | The live dot and the symbol's ring only. |

### Sizes, density and instrument marks

| Token | Value | Usage |
|---|---|---|
| `row-compact` | `32px` | Row height, Compact density (queues, event streams). |
| `row-default` | `40px` | Row height, Default density. |
| `control-sm` | `28px` | Buttons and inputs inside rows and toolbars. |
| `control-md` | `36px` | Default button, input and select height. |
| `trace-stroke` | `1.5px` | Stroke of every trace line and chart series. |
| `tick-minor` | `4px` | Length of minor ruler ticks. |
| `tick-major` | `10px` | Length of major ruler ticks (every 10 points). |

### Shadow

| Token | Value | Usage |
|---|---|---|
| `shadow-overlay` | dark: `0 12px 32px rgba(0, 0, 0, 0.55)` / light: `0 12px 32px rgba(21, 24, 22, 0.14), 0 0 0 1px rgba(21, 24, 22, 0.08)` | Menus, popovers, drawers, modals only. Panels on the page use line-subtle. |

## Implementation

The implementation lives in the frontend:

- Tokens: `frontend/app/globals.css` (CSS variables for both themes, mapped into Tailwind v4 with `@theme inline`, so components never need `dark:` variants).
- Risk bands: `frontend/lib/risk.ts` (bands, default thresholds, `bandFor`, `readout`), reading the environment's thresholds from `GET /api/v1/risk/policy`.
- Logo files: `frontend/public/brand/` (`riskos-symbol-on-dark.svg`, `riskos-symbol-on-light.svg`, `riskos-app-icon.svg`).
- Components: `frontend/components/instrument/` (`RiskScore`, `RiskBadge`, `ReasonList`, `TraceLine`, `Button` and the console primitives: panels, fields, tables with the selected-row needle, drawer, modal, states), on Radix primitives. The alert queue row lives in `frontend/features/alerts/`.

The token values in `globals.css` and the logo SVGs follow this document's tables and sources. `lib/risk.ts` implements the risk scale with the environment's thresholds instead of fixed ones. Where code and this document differ, this document wins.

## Logo files

The RiskOS symbol is a dial ring with a trace running through it: the instrument reading a spike. The trace always crosses the ring from edge to edge and is never shown without the ring.

- `riskos-symbol-on-dark.svg`: ring in paper `#eceee6`, trace in signal lime `#c8f03c`. Use it on `bg-canvas` / `bg-surface` in the dark theme.
- `riskos-symbol-on-light.svg`: ring in graphite `#151816`, trace in olive `#4a6400`. Use it on paper in the light theme. Lime is never used on paper.
- `riskos-app-icon.svg`: a graphite tile with a paper ring and lime trace. Use it for the favicon, app icon and social avatar.
- **Wordmark:** the symbol followed by `riskos` in lowercase Instrument Sans 600, letter-spacing -0.03em. Set the symbol height equal to the cap height plus 30%, with a gap of 0.3× the symbol width. Always lowercase in the wordmark, "RiskOS" in running text.
- Clear space is the ring's radius on every side. Minimum size is 16px (symbol) or 72px wide (wordmark). Don't rotate it, recolor the trace to a risk color, or animate the ring. The trace may draw on once at load.

## Components

### RiskScore

The headline reading for one event or case: a three-digit readout in a sunken well, the band badge, a 0–100 ruler with the needle at the score, the decision, and the evaluation latency.

- **Provide:** `score`, `decision` (`allow | review | step_up | block`), `policyId`, `latencyMs`, and `thresholds` from the tenant policy (defaults 20/50/70/90).
- **Tokens:** readout in the `readout` style with leading zeros in `ink-3`, on `bg-sunken` with `radius-0`. On the ruler, minor ticks are `line-subtle`, major ticks and numbers `ink-3` in `legend`, thresholds dashed `line-control`, and the band strip `risk-*` (bands above the score at 30%). The needle is a 2px `ink` line.
- **Motion:** the needle eases from 0 to the score once, over 180ms. Nothing else moves.
- **Do** place a `ReasonList` right after it. Use only one per view. In rows, use `RiskBadge` instead.
- Mark it up as `role="meter"` with a label such as "Risk score 87 of 100, High".

### RiskBadge

Shows a risk band three ways at once: a 1–5 bar glyph (like signal strength), the band word, and the band colour on its wash, followed by the score.

- **Provide:** `score` (0–100), or `band` (`minimal | low | elevated | high | critical`), plus an optional `showScore` (default true).
- **Tokens:** text and bars in `risk-<band>` on `risk-<band>-wash`, `radius-sm`, 22px tall, `label` type, with the score in mono.
- **Do** use it in every row, drawer header and export. The bars keep it readable in greyscale and print.
- **Don't** use a risk colour for anything that isn't a band, drop the word to save space, or round the corners further.

### ReasonList

The factors that moved a score, ranked by contribution and measured in ticks. This is RiskOS's explanation.

- **Provide:** `reasons[]` of `{ label, category, contribution }` from the API's explanation payload, plus `limit` (default 5, with "Show all").
- **Tokens:** ▲ in `risk-high` means a factor raises risk and ▼ in `risk-low` means it lowers it; the arrow carries the meaning. Contribution is a 12-tick bar (`ink-2` on, `line-subtle` off) plus signed points in `data`. Category is in `legend`.
- **Do** write labels as evidence, with numbers and windows. **Don't** show raw feature names outside the developer "raw" toggle.

### TraceLine

The brand's signature line: a thin sparkline, flat until it spikes. Use it for live throughput, per-entity activity in rows, and KPI tiles.

- **Provide:** `points[]`, plus optional `baseline` (a number drawn as a dashed line), `width`, `height` and `live` (adds the pulsing dot and label).
- **Tokens:** the stroke is `trace-stroke` in `signal-ink`, with miter joins and no fill. The baseline is a 1px dashed `ink-3` line. The live dot is 6px `signal-ink` with `radius-full`.
- **Motion:** it draws left to right once on mount. A live trace shifts left as points arrive. No easing bounce.
- **Don't** fill the area under it, smooth it into curves, or colour it by risk. For risk over time, use a chart with `risk-*` bands.

### AlertRow

One row of the alert queue: badge, event ID, evidence summary, a one-hour trace, amount and age.

- **Provide:** `alert` as `{ id, score, summary, amount, currency, createdAt, series1h[] }`, plus `selected` and `density` (`default` = `row-default`, `compact` = `row-compact`).
- **Tokens:** header in `legend` on `bg-sunken`, rows divided by `line-subtle`, hover `bg-raised`. A selected row gets `signal-wash` plus a 2px `signal-ink` needle bar at its left edge. The trace is `TraceLine` at 96×20. IDs, amounts and age are in `data`, with amounts right-aligned.
- **Do** sort by score, then by age. Support `j`/`k` to move and `Enter` to open the drawer. **Don't** tint the whole row by band.

### Button

Triggers an action. There are three variants.

- **Primary** uses `primary-fill` with an `on-primary` label: lime with a graphite label on dark, graphite with a lime label on paper. Use one per view, for the forward action ("Open case", "Create rule").
- **Secondary** is transparent with a `line-control` edge and an `ink` label. Use it for queue decisions, with the shortcut shown in a `kbd`.
- **Danger** has a `status-error` edge and label. Use it only for irreversible actions (Block account, Revoke key), and always follow it with a confirm step that names the object.
- **Provide:** `variant`, `size` (`sm` = `control-sm`, `md` = `control-md`), `shortcut?`, `icon?`, and a sentence-case verb as the label. Corners use `radius-md`. Focus is a 2px square `focus-ring`.

## Benchmark: security and fintech-risk design systems

This section records what the reference systems teach and what RiskOS takes from each. When a decision above looks arbitrary, check here for the reason behind it.

| System (company) | What it is known for | What RiskOS adopts |
|---|---|---|
| **Carbon** (IBM, used across IBM Security / QRadar) | Four themes (White, Gray 10, Gray 90, Gray 100). Dark layers step one shade lighter per layer. A single blue is the primary action color. IBM Plex typefaces. | The layered surfaces (`bg-canvas` → `bg-surface` → `bg-raised`), a single interaction colour, and dark as a first-class theme. |
| **EUI** (Elastic, powers Elastic Security / Kibana) | A formal severity pattern with six levels (Unknown, Success, Neutral, Warning, Risk, Danger). Each level has text and background tokens at three intensities. | A fixed, ordered risk scale with a paired `-wash` token per band. A distinct "minimal/unknown" grey so unscored events are not shown as safe. |
| **Radar** (Stripe) | A 0–99 risk score with named levels (normal, elevated, highest). Elevated goes to review and highest is blocked by default. A "risk insights" panel explains the factors behind each score. | Score + band + default decision always shown together. `ReasonList` as the standard explanation. Bands that map directly to routing. |
| **Odyssey** (Okta) | Accessibility-driven and open source. Every component meets WCAG 2.1 AA. Colors are inspired by currency and "timeless signifiers of identity and security". | AA as the floor for every token pair. A restrained, trust-first palette with no decorative gradients. |
| **DRUIDS** (Datadog) | 150+ React/TypeScript components treated as the source of truth. Consistency across products ("works one way here, the same way everywhere"). Command-K discovery. | One component per job, shared by console and docs. Keyboard-first navigation and a command palette. |
| **Blueprint** (Palantir) | Built for complex, data-dense desktop apps. Composable atomic components. Colorblind-aware palette with consistent contrast. Uniform keyboard behavior. | Composition over bespoke screens. Colorblind-safe scale (no red/green dependency). Keyboard shortcuts on every queue action. |
| **Kumo** (Cloudflare) | Open-source React library on Base UI (unstyled, accessible primitives) with Tailwind-registered theme tokens. | Implementation path: build RiskOS components on an accessible headless layer (Base UI or Radix) and expose these tokens as CSS variables / Tailwind theme. |

### Patterns common to all of them

- **Dark mode is expected** in analyst and SOC tools, but documentation and reports stay light.
- **Severity is a scale, not a set of alerts.** Each level has an ordered position, a name, a color and a default action.
- **Explainability is part of the UI.** Stripe's risk insights and Elastic's alert details both put the "why" next to the score.
- **Neutral-first palettes.** Brand color is scarce, so the severity colors are the loudest things on screen.
- **Accessibility is a stated requirement,** not a nice-to-have. That means AA contrast, keyboard coverage, and colorblind-safe encoding.

### Where RiskOS differs on purpose

The references above are strong, but they look alike: cool slate grounds, a blue or teal action colour, and generic grotesks. RiskOS keeps their good habits (layering, a fixed severity scale, explanations next to scores, AA contrast) and takes a different identity:

- **An idea instead of a palette.** "Instrument" (a seismograph for money) drives the type, colour, shapes, motion and logo. None of the benchmarks has a visual metaphor this specific.
- **Warm graphite and chart paper, not blue-slate and white.** This sets RiskOS apart from Carbon, Splunk and most SOC consoles at a glance.
- **Signal lime as the only brand colour.** It is rare in fintech, it reads as "live", and it sits outside the risk scale, so it never gets confused with a band. System health reuses it ("the line is live"), so no separate green is needed.
- **Low risk is blue and every band has a bar count.** This goes further than EUI's colour-only severity chips: bands stay readable in greyscale and for colour-blind analysts.
- **Readouts, rulers and traces instead of cards, rings and donuts.** The score looks like a meter reading, which fits the claim that RiskOS measures risk precisely.
- **Instrument Sans with JetBrains Mono,** instead of Plex (Carbon's font) or Inter (everyone's font).

### Sources

- [Carbon: color overview](https://carbondesignsystem.com/elements/color/overview/)
- [EUI: severity pattern](https://eui.elastic.co/docs/patterns/severity/)
- [Stripe Radar: risk evaluation](https://docs.stripe.com/radar/risk-evaluation)
- [Okta: introducing Odyssey](https://okta.com/blog/2021/02/okta-design-system)
- [Datadog: DRUIDS](https://datadoghq.com/blog/engineering/druids-the-design-system-that-powers-datadog)
- [Palantir: scaling product design with Blueprint](https://blog.palantir.com/scaling-product-design-with-blueprint-25492827bb4a)
- [Cloudflare Kumo: installation](https://kumo-ui.com/installation.md)
