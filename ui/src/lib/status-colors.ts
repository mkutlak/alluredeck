// Catppuccin Latte (light) hex colors
export const STATUS_COLORS = {
  passed: '#40a02b', // green
  failed: '#d20f39', // red
  broken: '#fe640b', // peach
  skipped: '#8c8fa1', // overlay1
} as const

// Catppuccin Mocha (dark) hex colors
export const STATUS_DARK_COLORS = {
  passed: '#a6e3a1', // green
  failed: '#f38ba8', // red
  broken: '#fab387', // peach
  skipped: '#a6adc8', // subtext1
} as const

// Tailwind text class strings (light text-[hex] dark:text-[hex])
// Note: skipped text uses subtext0 (#6c6f85) for better contrast, not overlay1
export const STATUS_TEXT_CLASSES = {
  passed: 'text-[#40a02b] dark:text-[#a6e3a1]',
  failed: 'text-[#d20f39] dark:text-[#f38ba8]',
  broken: 'text-[#fe640b] dark:text-[#fab387]',
  skipped: 'text-[#6c6f85] dark:text-[#a6adc8]',
} as const

// Background-only class strings (for non-badge elements like category spans)
export const STATUS_BG_CLASSES = {
  passed: 'bg-[#40a02b]/15 dark:bg-[#a6e3a1]/15',
  failed: 'bg-[#d20f39]/15 dark:bg-[#f38ba8]/15',
  broken: 'bg-[#fe640b]/15 dark:bg-[#fab387]/15',
  skipped: 'bg-[#8c8fa1]/15 dark:bg-[#7f849c]/15',
} as const

// Full CVA-compatible badge class strings (border + bg + text, light + dark)
export const STATUS_BADGE_CLASSES = {
  passed:
    'border-transparent bg-[#40a02b]/15 text-[#40a02b] dark:bg-[#a6e3a1]/15 dark:text-[#a6e3a1]',
  failed:
    'border-transparent bg-[#d20f39]/15 text-[#d20f39] dark:bg-[#f38ba8]/15 dark:text-[#f38ba8]',
  broken:
    'border-transparent bg-[#fe640b]/15 text-[#fe640b] dark:bg-[#fab387]/15 dark:text-[#fab387]',
  skipped:
    'border-transparent bg-[#8c8fa1]/15 text-[#6c6f85] dark:bg-[#7f849c]/15 dark:text-[#a6adc8]',
} as const

// Full CVA-compatible badge class string for informational (blue) content — light + dark
export const INFO_BADGE_CLASSES =
  'border-transparent bg-[#1e66f5]/15 text-[#1e66f5] dark:bg-[#89b4fa]/15 dark:text-[#89b4fa]'

// Full CVA-compatible badge class string for neutral content — reuses the skipped overlay grays
export const NEUTRAL_BADGE_CLASSES =
  'border-transparent bg-[#8c8fa1]/15 text-[#6c6f85] dark:bg-[#7f849c]/15 dark:text-[#a6adc8]'

// Accent (Catppuccin mauve) — used for trace/tooling badges that need a distinct hue
export const ACCENT_TEXT_CLASSES = 'text-[#8839ef] dark:text-[#cba6f7]'

// Full CVA-compatible badge class string for accent content — light + dark
export const ACCENT_BADGE_CLASSES =
  'border-transparent bg-[#8839ef]/15 text-[#8839ef] dark:bg-[#cba6f7]/15 dark:text-[#cba6f7]'

// Defect category badge classes — tinted (bg/15 + colored text + border-transparent), not solid
export const DEFECT_CATEGORY_BADGE_CLASSES: Record<
  'product_bug' | 'test_bug' | 'infrastructure' | 'to_investigate',
  string
> = {
  product_bug: STATUS_BADGE_CLASSES.failed,
  test_bug: STATUS_BADGE_CLASSES.broken,
  infrastructure: INFO_BADGE_CLASSES,
  to_investigate: NEUTRAL_BADGE_CLASSES,
}

// Category colors for charts
export const CATEGORY_COLORS: Record<string, string> = {
  'Product defects': '#d20f39', // catppuccin latte red
  'Test defects': '#fe640b', // catppuccin latte peach
}

export const CATEGORY_DEFAULT_COLOR = '#8c8fa1' // catppuccin latte overlay1

/**
 * Returns a Tailwind text color class based on pass rate thresholds.
 * >= 90: green (passed), >= 70: orange (broken), < 70: red (failed)
 */
export function getPassRateColorClass(rate: number): string {
  if (rate >= 90) return STATUS_TEXT_CLASSES.passed
  if (rate >= 70) return STATUS_TEXT_CLASSES.broken
  return STATUS_TEXT_CLASSES.failed
}

/**
 * Returns additional className for a pass-rate badge.
 * >= 90: green, >= 70: orange, < 70: undefined (use default destructive variant)
 */
export function getPassRateBadgeClass(rate: number): string | undefined {
  if (rate >= 90)
    return 'bg-[#40a02b] text-white hover:bg-[#40a02b]/90 dark:bg-[#a6e3a1] dark:text-[#1e1e2e] dark:hover:bg-[#a6e3a1]/90'
  if (rate >= 70)
    return 'bg-[#fe640b] text-white hover:bg-[#fe640b]/90 dark:bg-[#fab387] dark:text-[#1e1e2e] dark:hover:bg-[#fab387]/90'
  return undefined
}
