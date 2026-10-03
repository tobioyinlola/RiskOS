# RiskOS Design System

The source of truth for how RiskOS looks: the console, the internal admin console and the
marketing site share one light design system. Tokens live as CSS variables in
`frontend/app/globals.css` and reach components through the Tailwind theme. Components never
hard-code colors, spacing, radii, shadows or fonts.

The visual language (white cards floating on a light gray canvas, ink pill buttons, a blue
brand, bold headlines with a lighter italic accent, gray decimals on big figures) is adapted
from a fintech brand reference. The content and the risk semantics are RiskOS's own.

## Principles

1. **The score is never alone.** Every risk score ships with its band word, its decision and
   its top reasons. Show `RiskScore` with a `ReasonList`, never a bare number.
2. **Color carries meaning.** Brand blue is for interaction and the primary data series. The
   `risk-*` tokens are reserved for risk bands, `status-*` for system and workflow state. Red is
   the brand's second color, but in a risk product it means danger: use it for Critical,
   destructive actions and the unread dot, and sparingly anywhere else.
3. **Never color alone.** Every band, status and decision carries a word, a glyph, or both.
4. **Light only.** The product has one theme. There is no dark mode, theme toggle or
   `prefers-color-scheme` logic; `color-scheme: light` is set on `:root`.
5. **Evidence over decoration.** IDs, keys, hashes and payloads are mono and one click to copy.
   The console shows no illustrations; the marketing site may.
6. **Honest figures.** Every figure says what it covers ("last 100 events"). The product never
   shows sample or invented data; marketing mockups are marked `aria-hidden` and show no
   customer claims.

## Voice and content

- Write plainly, the way a senior analyst writes a case note: "Card used from 3 countries in
  40 minutes", not "Suspicious activity detected!"
- Sentence case everywhere, including buttons ("Open case"). No exclamation marks or emoji in
  the product UI; the overview greeting ("Welcome back, Ada!") is the one exception.
- Decisions are verbs: **Allow**, **Step-up**, **Review**, **Block**. Bands are adjectives:
  **Minimal**, **Low**, **Elevated**, **High**, **Critical**.
- Numbers carry units and scope: "4,500.00 NGN", "score 87 / 100", "last 100 events".

## Color

### Brand palette

| Token | Value | Use |
|---|---|---|
| `brand-blue` | `#335BFF` | ~35% of brand color: active navigation, links, the primary data series, focus rings |
| `brand-red` | `#FB3748` | Critical fills, the alerts dot, destructive emphasis. Sparingly |
| `brand-orange` | `#FA7319` | High fills, warning emphasis |
| `white` | `#FFFFFF` | Cards and surfaces |
| `ink` | `#0E121B` | Primary text, primary buttons, the active tab pill; the system's "black" |

### Derived

| Token | Value | Use |
|---|---|---|
| `blue-50` / `100` / `200` / `300` | `#EEF2FF` / `#DFE6FF` / `#C2CFFF` / `#96ACFF` | Active nav pill, selection, soft fills, decision bars |
| `blue-600` (`accent-hover`) | `#2547D0` | Hover blue; blue text on tints (6.5:1 on `blue-50`) |
| `bg-canvas` | `#F6F7F9` | The page |
| `bg-surface` | `#FFFFFF` | Cards, sidebar, menus |
| `bg-raised` / `bg-sunken` | `#F6F7F9` / `#F1F2F5` | Hover rows, insets, skeletons, code |
| `border-subtle` | `#E6E8EC` | Card edges and dividers |
| `border-strong` | `#E1E4EA` | Secondary buttons, pills, tabs |
| `border-control` | `#868C98` | Form controls (3:1 on white, WCAG 1.4.11) |
| `text-primary` | `#0E121B` | Body and headings (17.5:1 on canvas) |
| `text-secondary` | `#525866` | Supporting text (6.6:1 on canvas) |
| `text-muted` | `#5C6370` | Labels, captions, axes (5.6:1 on canvas) |
| `text-faint` | `#868C98` | The decimals of large figures and muted words in big statements. **Large text only** (3.4:1) |
| `chart-1` / `chart-2` | `#335BFF` / `#22B8B2` | The two series: blue, then teal |

### Risk scale and status

Each band pairs an AA text color with a tinted ground (badges, pills) and a vivid fill (bars,
meters, chart marks). Text on its ground is at least 4.5:1.

| Band | Score | Text | Ground | Fill |
|---|---|---|---|---|
| Minimal (Info) | 0–19 | `#525866` | `#F1F2F4` | `#B4B9C3` |
| Low | 20–49 | `#2547D0` | `#EEF2FF` | blue `#335BFF` |
| Elevated (Medium) | 50–69 | `#8A5A00` | `#FFF6E0` | amber `#F6B51E` |
| High | 70–89 | `#A44A05` | `#FFF1E6` | orange `#FA7319` |
| Critical | 90–100 | `#C2111F` | `#FFEBED` | red `#FB3748` |

Status tones (`StatusPill`): `success` (Resolved, Allow, Completed) `#16753F` on `#E7F7EE`,
`warning` (Open, Review, Pending) = High, `danger` (Block, Canceled) = Critical, `info`
(Investigating, Step-up) = Low, `neutral` = Minimal.

Tenants can move the band thresholds; the UI reads them from `GET /api/v1/risk/policy` and
shows the decision the policy returns ([RISK_ENGINE.md](RISK_ENGINE.md)).

Decisions in bars use the blue family, darker as the intervention grows (`DECISION_FILL`):
Allow `blue-200`, Step-up `blue-300`, Review `brand-blue`, Block `blue-600`.

### Landing washes

`bg-hero` (lavender `#ECE9FF` into cyan `#E3F6FB` into white), `bg-feature` (the pastel card
wash) and `bg-grid` (a faint 1px blue grid, 56px). `fade-radial` and `fade-bottom` mask layers.

## Typography

- **Typeface:** **Inter** (400, 500, 600, 700, with italics), loaded in `frontend/app/fonts.ts`
  and read everywhere through `--font-sans`.
- **Mono:** IBM Plex Mono with tabular figures, for IDs, keys, hashes and payloads only.
- **Headlines** are bold with tight tracking. The signature pattern is a bold phrase followed by
  a lighter italic phrase (`.accent-italic`): "Every decision, *explained.*" `PageHeader` takes
  an `accent`; the landing `Headline` does the same.
- **Big figures** (`Figure`) set their decimals in `text-faint`: "43.**1**". Use it at
  `metric` size or larger only.

| Style | Size / line | Weight | Tracking | Use |
|---|---|---|---|---|
| `hero` | 64 / 68 | 700 | −0.035em | Landing hero |
| `display` | 40 / 46 | 700 | −0.03em | Landing sections |
| `heading-1` | 28 / 34 | 700 | −0.02em | Page titles |
| `heading-2` | 20 / 28 | 600 | −0.01em | Drawer and modal titles |
| `heading-3` | 16 / 22 | 600 | | Card titles |
| `figure-xl`, `score-xl` | 40 / 44 | 700 | −0.03em | Headline figure, headline score |
| `metric` | 28 / 32 | 700 | −0.02em | Card figures |
| `body-lg` | 17 / 26 | 400 | | Landing paragraphs |
| `body` | 14 / 20 | 400 | | Default |
| `body-sm` | 13 / 18 | 400 | | Tables, captions |
| `label` | 12 / 16 | 500 | | Column headers, pills, form labels |
| `data` | 13 / 18 | mono | | IDs and keys |

## Shape, depth and spacing

- **Radius:** `sm` 6px (small tiles), `md` 12px (inner elements, inputs, list items), `lg` 20px
  (cards), `xl` 28px (landing panels, mockups), `full` (buttons, pills, tabs, nav items).
- **Shadow:** `shadow-card` (very soft, on every card), `shadow-overlay` (menus, drawers,
  modals, tooltips), `shadow-primary` (the ink button's inner highlight), `shadow-glow` (the
  hero accent chip).
- **Spacing:** an 8px rhythm with a 4px half step. Utilities 1–7 are the space tokens (4, 8,
  12, 16, 24, 32, 48); 8 and above are plain 4px steps. Cards pad 20px (`px-5`), sit 20px
  apart, and the page gutter is 32px on desktop.
- **Rows:** `row-default` 52px, `row-compact` 40px (a per-browser preference).

## Iconography

Phosphor, "regular" weight: a 1.5px line on the 24px grid with rounded caps and joins. The
active navigation item uses the "fill" weight. Import icons only from
`frontend/components/ui/icons.tsx`, which names them by meaning (`Alert`, `ShieldCheck`,
`Investigate`, `Live` …); ESLint rejects direct imports of icon libraries. Icons sit in subtle
rounded tiles (`IconTile`) beside card titles and list items. Sizes: `icon-sm` 16px inline,
`icon-md` 20px in navigation and icon buttons.

The logo is the name: "RiskOS" set bold with tight tracking, "OS" in brand blue (`Wordmark`).
There is no symbol. Where a square is needed (the workspace switcher), `Monogram` shows the
workspace's initials on a blue tile, with the name beside it.

## Components

| Component | File | Notes |
|---|---|---|
| `Button` | `ui/button.tsx` | Pills. `primary` (ink, inner highlight, `arrow` adds ↗), `secondary` (white, 1px edge, leading `icon`), `ghost`, `accent` (blue), `danger` |
| `Panel`, `PanelHeader`, `PanelBody` | `ui/panel.tsx` | The card |
| `PageHeader` | `ui/panel.tsx` | Muted `eyebrow` line, title with optional italic `accent`, description, actions |
| `Metric`, `IconTile` | `ui/panel.tsx` | KPI card; icon tile |
| `Figure` | `ui/figure.tsx` | Gray decimals |
| `StatusPill`, `DecisionPill`, `OutlinePill` | `ui/pill.tsx` | Soft pill with a dot; decisions; outlined tag |
| `RiskBadge`, `RiskScore`, `RiskMeter`, `ReasonList` | `ui/risk-*.tsx`, `ui/reason-list.tsx` | Bands with glyphs (circle, triangle, octagon) |
| `Tabs`, `pillTabClass` | `ui/misc.tsx` | Pill tabs: the active tab is the ink pill |
| `Table`, `Th`, `Tr`, `Td`, `DataCell`, `RowCheckbox` | `ui/table.tsx` | Muted header over a soft divider, row dividers |
| `SegmentedBar`, `SegmentLegend`, `TickScoreBar` | `ui/bars.tsx` | Movement bars; the barcode score bar |
| `SeriesChart`, `SeriesLegend` | `ui/line-chart.tsx` | Recharts: smooth blue and teal lines, dotted grid, active label bold, tooltip card |
| `Avatar`, `AvatarStack` | `ui/avatar.tsx` | Initials on a tint, or an icon |
| `Input`, `Select`, `Textarea`, `Field` | `ui/field.tsx` | 12px radius, blue focus halo |
| `Tooltip`, `Drawer`, `Modal`, `Confirm` | `ui/tooltip.tsx`, `ui/overlay.tsx` | Ink tooltips; floating drawer |
| `Sidebar`, `NavItem`, `NavGroup`, `WorkspaceSwitcher`, `PromoCard`, `MobileDrawer`, `SearchField` | `shell/app-shell.tsx` | The shell, shared by the console and the admin realm |
| `AuthFrame` | `shell/auth-frame.tsx` | Sign-in and sign-up: the form on the page (no card), a brand panel on the landing wash at the right |

Class names are merged with `cn` from `frontend/lib/cn.ts`, which knows the type-scale and
shadow tokens. The stock merger would treat `text-body` (a size) and `text-white` (a color) as
the same group and drop one of them.

## Layout

**App shell (console and admin).** A white 248px sidebar (collapsing to a 72px rail with `[`,
and to a drawer on small screens): the workspace switcher on top (monogram, organization, active
environment; the admin realm adds an ink "Admin" badge), the main navigation, labelled groups
("Workflows", "Organization"), and an onboarding card with a dark button at the foot. The
active item is a `blue-50` pill with blue text and the filled icon. The top bar sits on the
canvas: the environment switch, the search field with its ⌘K hint, density and shortcut
buttons, the alerts bell (a red dot while alerts are open) and the avatar menu.

**Overview.** A muted date line over "Welcome back, *Name*!", then a row of quick actions (the
first is the ink primary, the rest secondary with leading icons). Main column: the risk
overview card (headline figure with deltas, a summary sentence computed from the data, the two
series chart), two movement cards (interventions in the blue family, the risk mix in band
fills), and the activity table with pill tabs. Right column: open alerts (severity tiles,
masked identifiers) with a full-width secondary button, and the average-risk tick bar with
open cases (date, status pill, title, band).

**Admin.** The same shell, tokens and components; pages lean on the table card.

**Landing (`/home`).** A floating pill navbar; the hero on the lavender-to-cyan wash with the
grid, the italic accent word in a white chip with a blue glow, and the dashboard mockup fading
out; the overview statement with muted words and a phone mockup over a blue curve; a five-card
features bento; audience pill tabs with a collage; the footer with a large headline, link
columns, the newsletter field and a huge faint wordmark.

## Accessibility

WCAG 2.2 AA. Text tokens meet 4.5:1 on canvas and surface; band and status text meet 4.5:1 on
their grounds; `text-faint` is used only at 24px and above. Form controls have a 3:1 edge.
Every focusable element shows a 2px blue outline at 2px offset; inputs add a blue halo. Bands,
statuses and decisions always carry a word. Charts have text equivalents (an `aria-label`
summary, legends, and the figures beside them). Queue actions have keyboard shortcuts, listed
under `?`. Motion is 120–200ms ease-out and respects `prefers-reduced-motion`.

## Deviations from the brand reference

- Inter instead of the reference's Neue Montreal.
- A text wordmark instead of a symbol logo.
- The decimals gray (`#868C98`) is darker than a typical light gray so large figures keep 3:1.
- Form controls keep a darker edge than the reference's hairlines, for WCAG 1.4.11.
- No avatars of real people or "N+ people joined" counts: avatar stacks show the roles RiskOS
  serves. The hero uses a live HTML mockup instead of a screenshot.
- "Request a demo" reads "Get started" (to sign-in) until a demo destination exists.
- The newsletter field says plainly that it is not connected yet.
- The visibility (eye) toggle in the reference top bar is left out: the console has no masked
  values to reveal yet.
