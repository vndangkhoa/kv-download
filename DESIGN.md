# KV Download — Design System Specification (Astryx)

## Design System Authority
- **Framework**: `@astryxdesign/core` (`^0.7.16`) + `@astryxdesign/theme-neutral` (`^0.7.16`) + `@stylexjs/stylex` (`^0.19.2`).
- **Theme Definition**: `frontend/src/theme/kvDownloadTheme.ts` extending `neutralTheme` with dual-mode `[light, dark]` token tuples (`data-astryx-mode="light" | "dark"` via `<Theme theme={kvDownloadTheme} mode={effectiveTheme}>`).
- **Cascade Layer Order** (`frontend/src/app/layers.css`):
  `@layer reset, tw-preflight, astryx-base, astryx-theme;`
  Tailwind's default preflight is disabled in `tailwind.config.ts` (`corePlugins: { preflight: false }`) and wrapped in `@layer tw-preflight` (`frontend/src/theme/preflight.css`).

## Typography
- **Body & UI Sans**: `Hanken Grotesk`, system-ui fallback.
- **Heading**: `Hanken Grotesk`, system-ui fallback (semibold/bold, tight tracking).
- **Monospace**: `JetBrains Mono` strictly reserved for tabular metrics (sizes, percentages, speeds, ETAs), paths, and file hashes — never applied to prose inputs, dropdowns, or navigation labels.

## Dual-Mode Palette (WCAG AA+)
| Token | Light Mode | Dark Mode | Role |
|---|---|---|---|
| `--color-background-body` | `#f4f6fb` | `#0b0c12` | App viewport canvas |
| `--color-background-surface` | `#ffffff` | `#131520` | Header, sidebar, toolbar surfaces |
| `--color-background-card` | `#ffffff` | `#161926` | Task, group, and media cards |
| `--color-background-popover` | `#ffffff` | `#1c2030` | Menus, dialogs, tooltips |
| `--color-background-wash` | `#eef1f8` | `#0e1017` | Shell navigation backdrop |
| `--color-text-primary` | `#0f172a` | `#f1f5f9` | Primary titles, labels |
| `--color-text-secondary` | `#475569` | `#94a3b8` | Supporting metadata |
| `--color-border` | `rgba(15, 23, 42, 0.10)` | `rgba(255, 255, 255, 0.09)` | Hairline dividers |
| `--color-accent` | `#6d28d9` | `#8b5cf6` | Primary CTAs, active tabs |

## Android Port
- The native Android app (`android-app/`) ports these exact Light/Dark palette tokens and corner radii (`inner = 6.dp`, `element = 10.dp`, `container = 14.dp`) into `AstryxTheme.kt` for Jetpack Compose.
