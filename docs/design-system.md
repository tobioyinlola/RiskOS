# RiskOS Design System

> Spec for Claude Code. Use this file as the source of truth when building the RiskOS frontend (Next.js + TypeScript). Put it in the repo (e.g. `docs/design-system.md`) and reference it from `CLAUDE.md`. Implement the tokens as CSS variables in `app/globals.css`, map them into the Tailwind theme, and build components on an accessible headless layer (Radix or Base UI). Never hard-code colors, spacing or radii; always use the tokens below.

## Overview

RiskOS is an API-first financial risk intelligence platform: events go in, an explainable score and decision come out, and analysts work alerts and cases. This system is built for that job. Its users are fraud analysts, risk ops leads and developers integrating the API. They read dense data for hours, act on it quickly, and must be able to justify every decision later.

### Principles

1. **The score is never alone.** Every risk score ships with its band word, its decision and its top reasons. Show `RiskScore` with a `ReasonList` next to it, never a bare number.
2. **Color means risk or it means nothing.** The five `risk-*` tokens are reserved for risk bands. Brand teal (`accent`) is for interaction only. System health uses the `status-*` tokens. No decorative color.
3. **Never color alone.** Every band, status and decision carries a word, an icon, or both. The risk scale runs grey → blue → amber → orange → rose, so it never depends on telling red from green.
4. **Density is a setting.** Default to `row-default`; offer Compact (`row-compact`) on queues and event streams. Never go below 32px rows.
5. **Evidence over decoration.** Show IDs, timestamps and raw payloads in `data` (mono), and make them one click to copy. There are no illustrations in the product UI.
6. **Dark console, light documents.** The analyst console defaults to the `dark` theme. Docs, reports, exports and printed case files use `light`. Both themes are complete; users can switch.

### Voice and content

- Write plainly and precisely, the way a senior analyst writes a case note: "Card used from 3 countries in 40 minutes", not "Suspicious activity detected!"
- Use sentence case everywhere, including buttons ("Open case", "Mark as fraud"). Use uppercase only in the `overline` style.
- Speak to the user as "you". The product has no "I" or "we" in the UI. Use no exclamation marks and no emoji.
- Label decisions with verbs: **Allow**, **Review**, **Block**, **Step-up**. Label bands with adjectives: **Minimal**, **Low**, **Elevated**, **High**, **Critical**.
- Give numbers with units and scope: "4,500.00 NGN", "6× baseline (1h)", "score 87 / 100". Show relative time with the absolute time in a tooltip.
- Reasons read as evidence, not blame: "Device first seen 2 minutes ago", not "Fraudulent device".

### Risk scale

Scores are integers from 0 to 100. The bands and their default routing are fixed across the API, the UI and exports:

| Band | Score | Default decision | Text / fill | Badge ground |
|---|---|---|---|---|
| Minimal | 0–19 | Allow | `risk-minimal` | `risk-minimal-bg` |
| Low | 20–49 | Allow | `risk-low` | `risk-low-bg` |
| Elevated | 50–69 | Review | `risk-elevated` | `risk-elevated-bg` |
| High | 70–89 | Block or Step-up | `risk-high` | `risk-high-bg` |
| Critical | 90–100 | Block + open case | `risk-critical` | `risk-critical-bg` |

Tenants can move the thresholds. When they do, the UI reads the thresholds from the tenant's policy, never from hard-coded numbers, and shows a "custom thresholds" note beside the scale.

> **Implementation note (RiskOS engine 2.0.0):** the decisions an environment actually takes come from `GET /api/v1/risk/policy`, not from this table. By default the engine allows Minimal and Low (Low steps up with a strengthenable signal), reviews Elevated, steps up High (Review where step-up is impossible) and reviews Critical. Critical becomes Block only where the tenant switched automatic blocking on after shadow evidence ([RISK_ENGINE.md](RISK_ENGINE.md#automatic-blocking)). Show the decision the policy returns.

### Visual foundations

**Color.** Build screens from `bg-canvas` (page), `bg-surface` (panels) and `bg-raised` (menus, hover, selected row). Use `bg-sunken` for code, JSON and table header strips. Set text in `text-primary`, `text-secondary` and `text-muted`; all three pass 4.5:1 on every background in both themes. Divide with `border-subtle`. Give every input and secondary button a `border-control` edge (3:1). Fill primary actions with `accent` and put `on-accent` text on them. `accent-subtle` marks the selected row and active filters.

**Type.** Set the UI in IBM Plex Sans and all data in IBM Plex Mono with tabular figures (`font-variant-numeric: tabular-nums`), so amounts and scores line up in columns. Use `body` for the default text. Use `body-sm` in Compact tables. Use `label` for column headers and form labels. Use `score-xl` only for the one headline score on a case or event. Load both families from Google Fonts at weights 400, 500 and 600.

**Spacing and layout.** Everything sits on a 4px grid (`space-1` … `space-7`). Pad panels with `space-4`. Leave `space-5` between panels. Use `space-6` for page gutters. The standard layout is a left nav (240px), a filter bar, the main table, and a right drawer (480px) for the selected alert or case. The drawer opens over the table rather than replacing it, so the analyst never loses their place in the queue.

**Shape and depth.** Corners are small: `radius-sm` for badges, `radius-md` for controls, `radius-lg` for panels. Panels are flat with a `border-subtle` edge. Only floating layers (menus, popovers, drawers, modals) take `shadow-overlay`. There are no gradients, glows or glass effects.

**Focus and states.** Every focusable element gets a 2px solid `focus-ring` outline at 2px offset. Hover lifts a row to `bg-raised`. Disabled controls drop to 40% opacity and keep their border. Loading shows skeleton rows at the real row height, not spinners over tables.

**Motion.** Keep motion to 120–200ms ease-out for drawers, menus and row highlight. New events streaming into a table fade in without shifting rows the user is reading: pause the stream while the pointer is over the table. Respect `prefers-reduced-motion`.

**Data visualization.** Time series use `accent` for the primary series and `text-muted` for baseline or comparison. When a chart encodes risk, it uses the `risk-*` tokens in band order and nothing else. Score distributions are histograms with the band thresholds drawn as `border-control` rules. Every chart has a table view.

### Iconography

Use one outline icon set at a 1.5px stroke, sized `icon-sm` (16px) inline and `icon-md` (20px) in nav. Lucide is the suggested open-source set. Icons inherit the text color of their context. Required pairs: check for success, triangle for warning or elevated, octagon for block or critical, shield for policy, fingerprint for device, map-pin for geo, and clock for velocity. No logo has been supplied yet, so set the wordmark "RiskOS" in IBM Plex Sans 600 until one exists.

### Components

- `RiskBadge`: the band word on its tinted ground, in every table row and header.
- `RiskScore`: a number plus a 0–100 meter with band ticks, used for the headline score.
- `ReasonList`: ranked contributing factors with a direction and weight. This is how RiskOS answers "why".
- `AlertRow`: one row of the alert queue.
- `Button`: primary (`accent`), secondary (`border-control`), danger (`status-danger`, only for destructive actions like "Block account").

### Accessibility

Meet WCAG 2.2 AA everywhere. Every queue action (next alert, allow, review, block, assign) has a keyboard shortcut, shown in the tooltip and in a `?` overlay. Tables use real `<table>` markup with sortable headers announced to screen readers. Live event streams are announced politely and never steal focus.

## Tokens

Themes: `dark` (Console, the default for the analyst app) and `light` (Daylight, for docs, reports and exports). Set the theme with `data-theme` on `<html>`.

### Color

| Token | Dark | Light | Usage |
|---|---|---|---|
| `bg-canvas` | `#0b0f14` | `#f5f7f9` | Page background behind every panel. Never put data directly on it in dense views; use bg-surface. |
| `bg-surface` | `#121821` | `#ffffff` | Panels, tables, cards, the case drawer. The default ground for all text tokens. |
| `bg-raised` | `#1a222d` | `#ffffff` | Menus, popovers, hovered table rows, the selected row. In light it matches surface and relies on shadow-overlay. |
| `bg-sunken` | `#070a0e` | `#eceff3` | Code blocks, JSON payloads, table header strip, input wells. |
| `border-subtle` | `#232d3a` | `#e1e5ea` | Hairline dividers between rows and panels. Decorative only; never the only edge of a control. |
| `border-control` | `#5f6f83` | `#76859a` | Edges of inputs, selects, checkboxes, secondary buttons. Meets 3:1 on every background. |
| `text-primary` | `#e7ecf2` | `#0e1620` | Headings, table values, scores. On bg-canvas, bg-surface, bg-raised, bg-sunken. |
| `text-secondary` | `#a9b4c2` | `#445266` | Labels, column headers, descriptions. On all backgrounds. |
| `text-muted` | `#8593a5` | `#5b6879` | Timestamps, IDs, helper text, placeholder. On all backgrounds (4.5:1+). |
| `accent` | `#2dd4bf` | `#0d7066` | RiskOS Signal teal. Primary buttons, links, selected tab, active nav. Never used to mean a risk level. |
| `accent-hover` | `#5eead4` | `#0a5a52` | Hover and pressed state of accent fills and links. |
| `on-accent` | `#04201d` | `#ffffff` | Text and icons on an accent fill. |
| `accent-subtle` | `#123a37` | `#e3f3f1` | Selected row tint, active filter chip background. Text on it stays text-primary. |
| `focus-ring` | `#5eead4` | `#0d7066` | 2px solid outline, 2px offset, on every focusable element. 3:1+ on all backgrounds. |
| `risk-minimal` | `#94a3b8` | `#5b6879` | Score 0-19. Text/icon/meter fill on surfaces. Always paired with the word Minimal. |
| `risk-minimal-bg` | `#272e39` | `#eff0f2` | Badge background for Minimal; text is risk-minimal. |
| `risk-low` | `#6aa8ff` | `#1f5fd1` | Score 20-49. Blue, not green, so the scale never depends on red/green. Paired with the word Low. |
| `risk-low-bg` | `#202f45` | `#e9effa` | Badge background for Low; text is risk-low. |
| `risk-elevated` | `#f5b942` | `#9a5500` | Score 50-69. Default route: review queue. Paired with the word Elevated. |
| `risk-elevated-bg` | `#363226` | `#f5eee6` | Badge background for Elevated; text is risk-elevated. |
| `risk-high` | `#ff8a4c` | `#b93d0a` | Score 70-89. Default route: block or step-up. Paired with the word High. |
| `risk-high-bg` | `#382a28` | `#f8ece6` | Badge background for High; text is risk-high. |
| `risk-critical` | `#ff5c7a` | `#c0123c` | Score 90-100. Block and open a case. The only red in the product. Paired with the word Critical. |
| `risk-critical-bg` | `#38232f` | `#f9e7ec` | Badge background for Critical; text is risk-critical. |
| `status-success` | `#3ecf8e` | `#137a4b` | System health only: webhook delivered, key active, model healthy. Never a risk level. Always with a check icon. |
| `status-warning` | alias of `risk-elevated` | alias of `risk-elevated` | System warnings (rate limit near, model drift). Always with a warning icon. |
| `status-danger` | alias of `risk-critical` | alias of `risk-critical` | Destructive actions and system errors. Always with an error icon. |
| `status-info` | `#6aa8ff` | `#1f5fd1` | Neutral notices and inline tips. |

### Typography

Families (load IBM Plex Sans 400/500/600 and IBM Plex Mono 400/500 via `next/font/google`):

- `sans`: `"IBM Plex Sans", ui-sans-serif, system-ui, sans-serif`
- `mono`: `"IBM Plex Mono", ui-monospace, "SFMono-Regular", Menlo, monospace`

Use `font-variant-numeric: tabular-nums` on every mono style.

| Style | Family | Size / line height | Weight | Letter spacing | Usage |
|---|---|---|---|---|---|
| `display` | sans | 32px / 40px | 600 | -0.01em | Marketing and empty-state headlines only. |
| `heading-1` | sans | 24px / 32px | 600 | 0 | Page titles. |
| `heading-2` | sans | 18px / 26px | 600 | 0 | Panel and drawer titles. |
| `heading-3` | sans | 15px / 22px | 600 | 0 | Section titles inside panels. |
| `body` | sans | 14px / 20px | 400 | 0 | Default UI text, table cells, form values. |
| `body-sm` | sans | 13px / 18px | 400 | 0 | Dense tables, helper text, secondary metadata. |
| `label` | sans | 12px / 16px | 500 | 0 | Form labels, column headers, badge text. |
| `overline` | sans | 11px / 16px | 600 | 0.06em | Uppercase group labels in nav and KPI tiles. Use sparingly. |
| `score-xl` | mono | 40px / 44px | 500 | 0 | The headline risk score in a case or event header. |
| `metric` | mono | 24px / 28px | 500 | 0 | KPI tile numbers. |
| `data` | mono | 13px / 18px | 400 | 0 | IDs, amounts, hashes, IPs, timestamps, API keys, JSON. Tabular figures. |

### Spacing (4px grid)

| Token | Value | Usage |
|---|---|---|
| `space-1` | `4px` | Icon-to-label gap, badge vertical padding. |
| `space-2` | `8px` | Gap inside controls and between inline items; badge horizontal padding. |
| `space-3` | `12px` | Table cell horizontal padding; gap between form fields in compact density. |
| `space-4` | `16px` | Panel padding; gap between form fields. |
| `space-5` | `24px` | Gap between panels; drawer padding. |
| `space-6` | `32px` | Page gutters on desktop; section breaks. |
| `space-7` | `48px` | Empty states and page-top spacing only. |

### Radius

| Token | Value | Usage |
|---|---|---|
| `radius-sm` | `4px` | Badges, checkboxes, table row selection, tags. |
| `radius-md` | `6px` | Buttons, inputs, selects, menus. |
| `radius-lg` | `10px` | Panels, cards, drawers, modals. |
| `radius-full` | `999px` | Status dots, avatar, toggle track only. |

### Sizes and density

| Token | Value | Usage |
|---|---|---|
| `row-compact` | `32px` | Table row height in Compact density (alert queues, event streams). |
| `row-default` | `40px` | Table row height in Default density. |
| `control-sm` | `28px` | Small button/input height inside table rows and toolbars. |
| `control-md` | `36px` | Default button, input and select height. |
| `icon-sm` | `16px` | Inline icons in tables, badges, buttons. |
| `icon-md` | `20px` | Navigation and panel header icons. |

### Shadow

| Token | Value | Usage |
|---|---|---|
| `shadow-overlay` | dark: `0 8px 24px rgba(0, 0, 0, 0.5)` / light: `0 8px 24px rgba(14, 22, 32, 0.12), 0 0 0 1px rgba(14, 22, 32, 0.06)` | Menus, popovers, drawers, modals. Panels on the page use border-subtle, not shadow. |

## Implementation: `globals.css`

Paste into `app/globals.css`:

```css
:root,
[data-theme="dark"] {
  --bg-canvas: #0b0f14;
  --bg-surface: #121821;
  --bg-raised: #1a222d;
  --bg-sunken: #070a0e;
  --border-subtle: #232d3a;
  --border-control: #5f6f83;
  --text-primary: #e7ecf2;
  --text-secondary: #a9b4c2;
  --text-muted: #8593a5;
  --accent: #2dd4bf;
  --accent-hover: #5eead4;
  --on-accent: #04201d;
  --accent-subtle: #123a37;
  --focus-ring: #5eead4;
  --risk-minimal: #94a3b8;
  --risk-minimal-bg: #272e39;
  --risk-low: #6aa8ff;
  --risk-low-bg: #202f45;
  --risk-elevated: #f5b942;
  --risk-elevated-bg: #363226;
  --risk-high: #ff8a4c;
  --risk-high-bg: #382a28;
  --risk-critical: #ff5c7a;
  --risk-critical-bg: #38232f;
  --status-success: #3ecf8e;
  --status-warning: var(--risk-elevated);
  --status-danger: var(--risk-critical);
  --status-info: #6aa8ff;
  --shadow-overlay: 0 8px 24px rgba(0, 0, 0, 0.5);
}

[data-theme="light"] {
  --bg-canvas: #f5f7f9;
  --bg-surface: #ffffff;
  --bg-raised: #ffffff;
  --bg-sunken: #eceff3;
  --border-subtle: #e1e5ea;
  --border-control: #76859a;
  --text-primary: #0e1620;
  --text-secondary: #445266;
  --text-muted: #5b6879;
  --accent: #0d7066;
  --accent-hover: #0a5a52;
  --on-accent: #ffffff;
  --accent-subtle: #e3f3f1;
  --focus-ring: #0d7066;
  --risk-minimal: #5b6879;
  --risk-minimal-bg: #eff0f2;
  --risk-low: #1f5fd1;
  --risk-low-bg: #e9effa;
  --risk-elevated: #9a5500;
  --risk-elevated-bg: #f5eee6;
  --risk-high: #b93d0a;
  --risk-high-bg: #f8ece6;
  --risk-critical: #c0123c;
  --risk-critical-bg: #f9e7ec;
  --status-success: #137a4b;
  --status-warning: var(--risk-elevated);
  --status-danger: var(--risk-critical);
  --status-info: #1f5fd1;
  --shadow-overlay: 0 8px 24px rgba(14, 22, 32, 0.12), 0 0 0 1px rgba(14, 22, 32, 0.06);
}

:root {
  --space-1: 4px;
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-5: 24px;
  --space-6: 32px;
  --space-7: 48px;
  --radius-sm: 4px;
  --radius-md: 6px;
  --radius-lg: 10px;
  --radius-full: 999px;
  --row-compact: 32px;
  --row-default: 40px;
  --control-sm: 28px;
  --control-md: 36px;
  --icon-sm: 16px;
  --icon-md: 20px;
  --font-sans: "IBM Plex Sans", ui-sans-serif, system-ui, sans-serif;
  --font-mono: "IBM Plex Mono", ui-monospace, "SFMono-Regular", Menlo, monospace;
}

*:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.tabular { font-variant-numeric: tabular-nums; }
```

## Implementation: Tailwind theme

If the project uses Tailwind v3 (`tailwind.config.ts`); for Tailwind v4, declare the same mappings in an `@theme` block:

```ts
import type { Config } from 'tailwindcss'

export default {
  darkMode: ['class', '[data-theme="dark"]'],
  content: ['./app/**/*.{ts,tsx}', './components/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        'bg-canvas': 'var(--bg-canvas)',
        'bg-surface': 'var(--bg-surface)',
        'bg-raised': 'var(--bg-raised)',
        'bg-sunken': 'var(--bg-sunken)',
        'border-subtle': 'var(--border-subtle)',
        'border-control': 'var(--border-control)',
        'text-primary': 'var(--text-primary)',
        'text-secondary': 'var(--text-secondary)',
        'text-muted': 'var(--text-muted)',
        'accent': 'var(--accent)',
        'accent-hover': 'var(--accent-hover)',
        'on-accent': 'var(--on-accent)',
        'accent-subtle': 'var(--accent-subtle)',
        'focus-ring': 'var(--focus-ring)',
        'risk-minimal': 'var(--risk-minimal)',
        'risk-minimal-bg': 'var(--risk-minimal-bg)',
        'risk-low': 'var(--risk-low)',
        'risk-low-bg': 'var(--risk-low-bg)',
        'risk-elevated': 'var(--risk-elevated)',
        'risk-elevated-bg': 'var(--risk-elevated-bg)',
        'risk-high': 'var(--risk-high)',
        'risk-high-bg': 'var(--risk-high-bg)',
        'risk-critical': 'var(--risk-critical)',
        'risk-critical-bg': 'var(--risk-critical-bg)',
        'status-success': 'var(--status-success)',
        'status-warning': 'var(--status-warning)',
        'status-danger': 'var(--status-danger)',
        'status-info': 'var(--status-info)'
      },
      spacing: {
        '1': 'var(--space-1)',
        '2': 'var(--space-2)',
        '3': 'var(--space-3)',
        '4': 'var(--space-4)',
        '5': 'var(--space-5)',
        '6': 'var(--space-6)',
        '7': 'var(--space-7)'
      },
      borderRadius: {
        'sm': 'var(--radius-sm)',
        'md': 'var(--radius-md)',
        'lg': 'var(--radius-lg)',
        'full': 'var(--radius-full)'
      },
      fontFamily: {
        sans: ['var(--font-sans)'],
        mono: ['var(--font-mono)'],
      },
      boxShadow: { overlay: 'var(--shadow-overlay)' },
    },
  },
} satisfies Config
```

Keep the risk band thresholds in one shared module (e.g. `lib/risk.ts`) and read tenant overrides from the policy API:

```ts
export type RiskBand = 'minimal' | 'low' | 'elevated' | 'high' | 'critical'
export const DEFAULT_THRESHOLDS = { low: 20, elevated: 50, high: 70, critical: 90 }
export function bandFor(score: number, t = DEFAULT_THRESHOLDS): RiskBand {
  if (score >= t.critical) return 'critical'
  if (score >= t.high) return 'high'
  if (score >= t.elevated) return 'elevated'
  if (score >= t.low) return 'low'
  return 'minimal'
}
```

## Components

### RiskBadge

Shows a risk band as a word (and optionally the score) on the band's tinted ground.

- **Provide:** `score` (0–100) or `band` (`minimal | low | elevated | high | critical`), and optional `showScore`.
- **Tokens:** text `risk-<band>` on `risk-<band>-bg`, `radius-sm`, `label` type, `space-1`/`space-2` padding.
- **Icon:** circle for Minimal/Low, triangle for Elevated/High, octagon for Critical, so the band reads without color.
- **Do** use it in every table row and case header. **Don't** use risk colors for anything that isn't a band, and don't abbreviate the band word.

Reference markup (static preview; tokens only):

```html
<!doctype html><html><head><meta charset="utf-8"><link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet"><style>body{margin:0;background:var(--bg-surface);color:var(--text-primary);font-family:var(--font-sans);font-size:14px;line-height:20px}.wrap{padding:var(--space-4);display:flex;flex-wrap:wrap;gap:var(--space-3);align-items:center}.mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.badge{display:inline-flex;align-items:center;gap:var(--space-1);padding:2px var(--space-2);border-radius:var(--radius-sm);font-size:12px;line-height:16px;font-weight:500}
.badge svg{width:12px;height:12px;fill:none;stroke:currentColor;stroke-width:2}
.minimal{color:var(--risk-minimal);background:var(--risk-minimal-bg)}.low{color:var(--risk-low);background:var(--risk-low-bg)}
.elevated{color:var(--risk-elevated);background:var(--risk-elevated-bg)}.high{color:var(--risk-high);background:var(--risk-high-bg)}
.critical{color:var(--risk-critical);background:var(--risk-critical-bg)}
</style></head><body><div class="wrap">
<span class="badge minimal"><svg viewBox="0 0 12 12"><circle cx="6" cy="6" r="4"/></svg>Minimal · 8</span>
<span class="badge low"><svg viewBox="0 0 12 12"><circle cx="6" cy="6" r="4"/></svg>Low · 31</span>
<span class="badge elevated"><svg viewBox="0 0 12 12"><path d="M6 1.5 11 10.5H1Z"/></svg>Elevated · 62</span>
<span class="badge high"><svg viewBox="0 0 12 12"><path d="M6 1.5 11 10.5H1Z"/></svg>High · 81</span>
<span class="badge critical"><svg viewBox="0 0 12 12"><path d="M4 1h4l3 3v4l-3 3H4L1 8V4Z"/></svg>Critical · 96</span>
</div></body></html>
```

### RiskScore

The headline score for one event or case: the number, its band, a 0–100 meter split at the band thresholds, and the decision taken.

- **Provide:** `score`, `decision` (`allow | review | block | step_up`), `policyId`, and the tenant's `thresholds` (defaults 20/50/70/90).
- **Tokens:** number in `score-xl` colored `risk-<band>`; meter segments `risk-*` (segments above the score at 35% opacity); marker `text-primary`; legend `text-muted` in `data`.
- **Do** place a `ReasonList` directly beside or beneath it. **Don't** use it in table rows (use `RiskBadge`), and never show a score without its band word.
- Expose it as `role="meter"` with an `aria-label` like "Risk score 81, High".

Reference markup (static preview; tokens only):

```html
<!doctype html><html><head><meta charset="utf-8"><link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet"><style>body{margin:0;background:var(--bg-surface);color:var(--text-primary);font-family:var(--font-sans);font-size:14px;line-height:20px}.wrap{padding:var(--space-4);display:flex;flex-wrap:wrap;gap:var(--space-3);align-items:center}.mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.card{padding:var(--space-4);width:320px}
.top{display:flex;align-items:baseline;gap:var(--space-2)}
.num{font-family:var(--font-mono);font-size:40px;line-height:44px;font-weight:500;color:var(--risk-high)}
.of{color:var(--text-muted);font-family:var(--font-mono);font-size:13px}
.band{margin-left:auto;font-size:12px;font-weight:500;color:var(--risk-high);background:var(--risk-high-bg);padding:2px 8px;border-radius:var(--radius-sm)}
.meter{position:relative;display:flex;gap:2px;margin-top:var(--space-3);height:8px}
.seg{border-radius:2px;opacity:.35}.on{opacity:1}
.s1{flex:20;background:var(--risk-minimal)}.s2{flex:30;background:var(--risk-low)}.s3{flex:20;background:var(--risk-elevated)}.s4{flex:20;background:var(--risk-high)}.s5{flex:11;background:var(--risk-critical)}
.pin{position:absolute;left:81%;top:-4px;width:2px;height:16px;background:var(--text-primary)}
.legend{position:relative;height:16px;margin-top:var(--space-2);font-size:12px;color:var(--text-muted)}.legend span{position:absolute;transform:translateX(-50%)}.legend span:first-child{transform:none}.legend span:last-child{transform:translateX(-100%)}
.dec{margin-top:var(--space-3);font-size:13px;color:var(--text-secondary)}.dec b{color:var(--text-primary);font-weight:600}
</style></head><body><div class="card">
<div class="top"><span class="num">81</span><span class="of">/ 100</span><span class="band">High</span></div>
<div class="meter" role="meter" aria-valuemin="0" aria-valuemax="100" aria-valuenow="81" aria-label="Risk score 81, High">
<span class="seg s1 on"></span><span class="seg s2 on"></span><span class="seg s3 on"></span><span class="seg s4 on"></span><span class="seg s5"></span><span class="pin"></span></div>
<div class="legend mono"><span style="left:0">0</span><span style="left:20%">20</span><span style="left:50%">50</span><span style="left:70%">70</span><span style="left:90%">90</span><span style="left:100%">100</span></div>
<div class="dec">Decision: <b>Step-up</b> · policy <span class="mono">card_txn_v4</span></div>
</div></body></html>
```

### ReasonList

Lists the factors that moved a score, ranked by contribution. This is RiskOS's explainability surface, the equivalent of Stripe's "risk insights".

- **Provide:** `reasons[]` of `{ label, category, contribution }` from the API's explanation payload (signed points or SHAP-style weights), plus optional `limit` (default 5, with "Show all").
- **Tokens:** ▲ in `risk-high` for factors that raise risk, ▼ in `risk-low` for factors that lower it (the arrow carries the meaning, color only reinforces it); weight bar `text-secondary` on `bg-sunken`; weights in `data`.
- **Do** write labels as evidence in plain language with numbers. **Don't** show raw feature names (`f_vel_1h`) except in a developer "raw" toggle.

Reference markup (static preview; tokens only):

```html
<!doctype html><html><head><meta charset="utf-8"><link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet"><style>body{margin:0;background:var(--bg-surface);color:var(--text-primary);font-family:var(--font-sans);font-size:14px;line-height:20px}.wrap{padding:var(--space-4);display:flex;flex-wrap:wrap;gap:var(--space-3);align-items:center}.mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.list{width:420px;padding:var(--space-4)}
h3{margin:0 0 var(--space-2);font-size:15px;line-height:22px;font-weight:600}
.row{display:grid;grid-template-columns:16px 1fr 84px 40px;gap:var(--space-2);align-items:center;padding:6px 0;border-top:1px solid var(--border-subtle)}
.dir{font-family:var(--font-mono);font-weight:500}.up{color:var(--risk-high)}.down{color:var(--risk-low)}
.txt{font-size:13px;line-height:18px}.txt small{display:block;color:var(--text-muted);font-size:12px}
.bar{height:6px;background:var(--bg-sunken);border-radius:3px;overflow:hidden}.bar i{display:block;height:100%;background:var(--text-secondary)}
.w{font-family:var(--font-mono);font-size:12px;color:var(--text-secondary);text-align:right}
</style></head><body><div class="list">
<h3>Why this score</h3>
<div class="row"><span class="dir up" aria-label="raises risk">▲</span><span class="txt">Card used in 3 countries in 40 min<small>velocity · geo</small></span><span class="bar"><i style="width:92%"></i></span><span class="w">+34</span></div>
<div class="row"><span class="dir up" aria-label="raises risk">▲</span><span class="txt">Device first seen 2 min ago<small>device</small></span><span class="bar"><i style="width:60%"></i></span><span class="w">+22</span></div>
<div class="row"><span class="dir up" aria-label="raises risk">▲</span><span class="txt">Amount 6× customer baseline<small>behavior</small></span><span class="bar"><i style="width:45%"></i></span><span class="w">+17</span></div>
<div class="row"><span class="dir down" aria-label="lowers risk">▼</span><span class="txt">Account age 4 years<small>identity</small></span><span class="bar"><i style="width:20%"></i></span><span class="w">−8</span></div>
</div></body></html>
```

### AlertRow

One row of the alert queue: band, event ID, a one-line evidence summary, amount and age.

- **Provide:** `alert` `{ id, score, summary, amount, currency, createdAt, assignee? }`, `selected`, `density` (`default` = `row-default`, `compact` = `row-compact`).
- **Tokens:** header `label` on `bg-sunken` in `text-secondary`; rows divided by `border-subtle`; hover `bg-raised`; selected `accent-subtle`; IDs and amounts in `data`, right-aligned amounts; age in `text-muted`.
- **Do** sort by score then age by default, and keep `j`/`k` for next/previous and `Enter` to open the drawer. **Don't** color the whole row by band; the badge carries it.

Reference markup (static preview; tokens only):

```html
<!doctype html><html><head><meta charset="utf-8"><link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet"><style>body{margin:0;background:var(--bg-surface);color:var(--text-primary);font-family:var(--font-sans);font-size:14px;line-height:20px}.wrap{padding:var(--space-4);display:flex;flex-wrap:wrap;gap:var(--space-3);align-items:center}.mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
table{border-collapse:collapse;width:100%;font-size:13px}
th{background:var(--bg-sunken);color:var(--text-secondary);font-size:12px;font-weight:500;text-align:left;padding:0 var(--space-3);height:32px}
td{padding:0 var(--space-3);height:40px;border-top:1px solid var(--border-subtle)}
tr.sel td{background:var(--accent-subtle)}tr:hover td{background:var(--bg-raised)}
.b{font-size:12px;font-weight:500;padding:2px 8px;border-radius:var(--radius-sm)}
.c{color:var(--risk-critical);background:var(--risk-critical-bg)}.e{color:var(--risk-elevated);background:var(--risk-elevated-bg)}
.m{color:var(--text-muted)}.r{text-align:right}
</style></head><body><table>
<thead><tr><th>Risk</th><th>Event</th><th>Summary</th><th class="r">Amount</th><th>Age</th></tr></thead>
<tbody><tr class="sel"><td><span class="b c">Critical · 96</span></td><td class="mono">evt_01J9ZK4Q7M</td><td>New device, 3 countries in 40 min</td><td class="mono r">4,500.00 NGN</td><td class="m">12s</td></tr>
<tr><td><span class="b e">Elevated · 58</span></td><td class="mono">evt_01J9ZJX2PA</td><td>Payee added and paid within 5 min</td><td class="mono r">120,000.00 NGN</td><td class="m">3m</td></tr></tbody></table></body></html>
```

### Button

Triggers an action. RiskOS has three variants.

- **Primary:** `accent` fill, `on-accent` text. Use one per view, for the main forward action ("Open case", "Create rule").
- **Secondary:** transparent with a `border-control` edge and `text-primary` label. Use it for queue decisions (Allow, Review, Step-up), and show the keyboard shortcut in `data` at `text-muted`.
- **Danger:** `status-danger` outline and label, reserved for irreversible or customer-impacting actions (Block account, Revoke key). It always opens a confirm step that names the object.
- **Provide:** `variant`, `size` (`sm` = `control-sm` in tables and toolbars, `md` = `control-md`), `shortcut?`, `icon?`, and a sentence-case verb label.
- The focus state is a 2px `focus-ring` outline at 2px offset. The disabled state is 40% opacity.

Reference markup (static preview; tokens only):

```html
<!doctype html><html><head><meta charset="utf-8"><link rel="preconnect" href="https://fonts.googleapis.com"><link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet"><style>body{margin:0;background:var(--bg-surface);color:var(--text-primary);font-family:var(--font-sans);font-size:14px;line-height:20px}.wrap{padding:var(--space-4);display:flex;flex-wrap:wrap;gap:var(--space-3);align-items:center}.mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}
button{font:500 14px/20px var(--font-sans);height:36px;padding:0 var(--space-4);border-radius:var(--radius-md);cursor:pointer;border:1px solid transparent}
button:focus-visible{outline:2px solid var(--focus-ring);outline-offset:2px}
.p{background:var(--accent);color:var(--on-accent)}.p:hover{background:var(--accent-hover)}
.s{background:transparent;color:var(--text-primary);border-color:var(--border-control)}.s:hover{background:var(--bg-raised)}
.d{background:transparent;color:var(--status-danger);border-color:var(--status-danger)}
.k{font-family:var(--font-mono);font-size:11px;color:var(--text-muted);margin-left:6px}
</style></head><body><div class="wrap">
<button class="p">Open case</button>
<button class="s">Allow<span class="k">A</span></button>
<button class="s">Send to review<span class="k">R</span></button>
<button class="d">Block account</button>
</div></body></html>
```

## Benchmark: security and fintech-risk design systems

This section records what the reference systems teach and what RiskOS takes from each. When a decision above looks arbitrary, check here for the reason behind it.

| System (company) | What it is known for | What RiskOS adopts |
|---|---|---|
| **Carbon** (IBM, used across IBM Security / QRadar) | Four themes (White, Gray 10, Gray 90, Gray 100). Dark layers step one shade lighter per layer. A single blue is the primary action color. IBM Plex typefaces. | The layered surfaces (`bg-canvas` → `bg-surface` → `bg-raised`), a single interaction color, IBM Plex Sans and Mono, dark as a first-class theme. |
| **EUI** (Elastic, powers Elastic Security / Kibana) | A formal severity pattern with six levels (Unknown, Success, Neutral, Warning, Risk, Danger). Each level has text and background tokens at three intensities. | A fixed, ordered risk scale with a paired `-bg` token per band. A distinct "minimal/unknown" grey so unscored events are not shown as safe. |
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

- **Low risk is blue, not green.** Most fraud tools use green→red, which fails for red-green colorblind analysts. Green is reserved for system health (`status-success`).
- **Teal brand, not blue.** Blue is taken by the Low band, and purple-blue is the generic fintech default. Signal teal stays clear of every band.
- **Mono for evidence.** IDs, amounts, IPs and payloads use IBM Plex Mono with tabular figures, because analysts compare values across rows.

### Sources

- [Carbon: color overview](https://carbondesignsystem.com/elements/color/overview/)
- [EUI: severity pattern](https://eui.elastic.co/docs/patterns/severity/)
- [Stripe Radar: risk evaluation](https://docs.stripe.com/radar/risk-evaluation)
- [Okta: introducing Odyssey](https://okta.com/blog/2021/02/okta-design-system)
- [Datadog: DRUIDS](https://datadoghq.com/blog/engineering/druids-the-design-system-that-powers-datadog)
- [Palantir: scaling product design with Blueprint](https://blog.palantir.com/scaling-product-design-with-blueprint-25492827bb4a)
- [Cloudflare Kumo: installation](https://kumo-ui.com/installation.md)

## Implementation in RiskOS

How the console (`frontend/`) applies this document. Where code and this document differ, this document wins.

**Files.**

- **Tokens:** `app/globals.css` holds both themes as CSS variables and maps them into Tailwind v4 with `@theme inline`. Utility names follow the token names, so a token becomes `bg-bg-surface`, `text-text-primary`, `border-border-subtle`, `text-accent`, `bg-risk-high-bg` and so on.
- **Spacing:** `p-1` … `p-7` (and every other spacing utility) resolve to `space-1` … `space-7`.
- **Sizes:** `h-row-default`, `h-control-md`, `size-icon-sm` and `size-icon-md` map to the size tokens.
- **Type styles:** `text-display`, `text-heading-1` … `text-heading-3`, `text-body`, `text-body-sm`, `text-label`, `text-score-xl`, `text-metric` and `text-data`. Add `.mono` for IBM Plex Mono with tabular figures. The `overline` style is `text-overline uppercase text-text-secondary`; there is no `.overline` class, because Tailwind's `overline` utility draws a line over the text.
- **Fonts:** IBM Plex Sans 400/500/600 and IBM Plex Mono 400/500, loaded with `next/font/google` in `app/layout.tsx`.
- **Components:** `components/ui/` holds `RiskBadge`, `RiskScore`, `ReasonList`, `Button`, the table parts (`Table`, `Th`, `Tr`, `Td`, `DataCell`), `Drawer`, `Modal`, form fields, states and `CopyValue`. They are built on Radix primitives. The alert queue row lives in `features/alerts/`.
- **Risk scale:** `lib/risk.ts` holds the bands and `bandFor`. The environment's thresholds and decisions come from `GET /api/v1/risk/policy`.

**Console shell** (`components/shell/`):

- **Left navigation:** 240px, collapsible to a 64px icon rail with `[` or the collapse button. Collapsed items show their name in a tooltip. Narrow screens get a drawer instead.
- **Command palette:** `Ctrl+K` / `⌘K` jumps to any section and runs console actions (switch environment, theme, density, sidebar, sign out).
- **Preferences:** theme (Console dark by default, Daylight light), table density (Default or Compact) and the sidebar state are kept per browser. The theme is applied before the first paint.
- **Production banner:** a standing banner whenever the production environment is selected.

**Queue actions.** Analysts triage alerts; the engine makes the decisions. The alert queue's shortcuts are `j`/`k` (move), `Enter` (open), `i` (investigating), `f` (false positive), `x` (resolve), `c` (open case) and `?` (help). There is no manual Allow/Review/Block until manual decisions exist.

**Logo.** None has been supplied, so the console uses the wordmark "RiskOS" in IBM Plex Sans 600 and a plain monogram favicon.
