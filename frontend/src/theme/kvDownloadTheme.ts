import { defineTheme } from '@astryxdesign/core/theme';
import { neutralTheme } from '@astryxdesign/theme-neutral/built';

export const kvDownloadTheme = defineTheme({
  extends: neutralTheme,
  name: 'kv-download',
  typography: {
    scale: { base: 14, ratio: 1.16 },
    body: {
      family: 'Hanken Grotesk',
      fallbacks: 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
    },
    heading: {
      family: 'Hanken Grotesk',
      fallbacks: 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
    },
    code: {
      family: 'JetBrains Mono',
      fallbacks: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    },
  },
  tokens: {
    // Surfaces: body -> surface -> card -> popover
    '--color-background-body': ['#f4f6fb', '#0b0c12'],
    '--color-background-surface': ['#ffffff', '#131520'],
    '--color-background-card': ['#ffffff', '#161926'],
    '--color-background-popover': ['#ffffff', '#1c2030'],
    '--color-background-muted': ['#f1f4f9', '#1a1d2b'],
    '--color-background-inverted': ['#0f172a', '#f8fafc'],
    '--color-overlay': ['rgba(15, 23, 42, 0.45)', 'rgba(4, 5, 10, 0.78)'],
    '--color-overlay-hover': ['rgba(15, 23, 42, 0.05)', 'rgba(241, 245, 249, 0.06)'],
    '--color-overlay-pressed': ['rgba(15, 23, 42, 0.10)', 'rgba(241, 245, 249, 0.11)'],
    '--color-tint-hover': ['rgba(109, 40, 217, 0.08)', 'rgba(139, 92, 246, 0.12)'],
    '--color-neutral': ['rgba(15, 23, 42, 0.07)', 'rgba(241, 245, 249, 0.08)'],
    '--color-skeleton': ['#e2e8f0', '#1e2233'],
    '--color-shadow': ['rgba(15, 23, 42, 0.12)', 'rgba(2, 4, 10, 0.65)'],

    // Primary Accent (Deep Violet) — WCAG AA+ contrast on both light and dark
    '--color-accent': ['#6d28d9', '#8b5cf6'],
    '--color-accent-muted': ['rgba(109, 40, 217, 0.10)', 'rgba(139, 92, 246, 0.16)'],
    '--color-on-accent': ['#ffffff', '#ffffff'],
    '--color-text-accent': ['#5b21b6', '#c4b5fd'],
    '--color-icon-accent': ['#6d28d9', '#a78bfa'],

    // Text & Icons
    '--color-text-primary': ['#0f172a', '#f1f5f9'],
    '--color-text-secondary': ['#475569', '#94a3b8'],
    '--color-text-disabled': ['#94a3b8', '#475569'],
    '--color-icon-primary': ['#0f172a', '#f1f5f9'],
    '--color-icon-secondary': ['#475569', '#94a3b8'],
    '--color-icon-disabled': ['#94a3b8', '#475569'],
    '--color-on-dark': ['#f8fafc', '#f8fafc'],
    '--color-on-light': ['#0f172a', '#0f172a'],

    // Borders
    '--color-border': ['rgba(15, 23, 42, 0.10)', 'rgba(255, 255, 255, 0.09)'],
    '--color-border-emphasized': ['rgba(15, 23, 42, 0.22)', 'rgba(255, 255, 255, 0.20)'],

    // Semantic Statuses
    '--color-success': ['#059669', '#10b981'],
    '--color-on-success': ['#ffffff', '#052e16'],
    '--color-success-muted': ['rgba(5, 150, 105, 0.12)', 'rgba(16, 185, 129, 0.16)'],
    '--color-on-success-muted': ['#065f46', '#6ee7b7'],
    '--color-text-success': ['#047857', '#34d399'],

    '--color-info': ['#0284c7', '#0ea5e9'],
    '--color-on-info': ['#ffffff', '#ffffff'],
    '--color-info-muted': ['rgba(2, 132, 199, 0.10)', 'rgba(56, 189, 248, 0.15)'],
    '--color-on-info-muted': ['#0369a1', '#7dd3fc'],
    '--color-text-info': ['#0369a1', '#38bdf8'],

    '--color-warning': ['#d97706', '#f59e0b'],
    '--color-on-warning': ['#ffffff', '#1c1917'],
    '--color-warning-muted': ['rgba(217, 119, 6, 0.12)', 'rgba(251, 191, 36, 0.15)'],
    '--color-on-warning-muted': ['#92400e', '#fcd34d'],
    '--color-text-warning': ['#b45309', '#fbbf24'],

    '--color-error': ['#dc2626', '#ef4444'],
    '--color-on-error': ['#ffffff', '#ffffff'],
    '--color-error-muted': ['rgba(220, 38, 38, 0.10)', 'rgba(248, 113, 113, 0.15)'],
    '--color-on-error-muted': ['#991b1b', '#fca5a5'],
    '--color-text-error': ['#b91c1c', '#f87171'],

    // Radii
    '--radius-inner': '6px',
    '--radius-element': '10px',
    '--radius-container': '14px',
    '--radius-page': '18px',

    // Shadows (offset + soft blur, never zero-offset decoration)
    '--shadow-low': ['0 2px 8px -2px rgba(15, 23, 42, 0.08)', '0 4px 12px -4px rgba(0, 0, 0, 0.55)'],
    '--shadow-med': ['0 10px 28px -8px rgba(15, 23, 42, 0.12)', '0 14px 34px -10px rgba(0, 0, 0, 0.68)'],
    '--shadow-high': ['0 20px 50px -14px rgba(15, 23, 42, 0.18)', '0 24px 60px -16px rgba(0, 0, 0, 0.80)'],
  },
});
