import type { CategoryEntry, ReportHistoryEntry, TimelineTestCase } from '@/types/api'
import type { ChartConfig } from '@/components/ui/chart'
import { calcPassRate } from './utils'

// ChartConfig objects for each chart type — use CSS variables for theming
export const statusChartConfig = {
  passed: { label: 'Passed', color: 'var(--chart-1)' },
  failed: { label: 'Failed', color: 'var(--chart-2)' },
  broken: { label: 'Broken', color: 'var(--chart-3)' },
  skipped: { label: 'Skipped', color: 'var(--chart-4)' },
} satisfies ChartConfig

export const passRateChartConfig = {
  passRate: { label: 'Pass Rate', color: 'var(--chart-5)' },
} satisfies ChartConfig

export const durationChartConfig = {
  durationSec: { label: 'Duration', color: 'var(--chart-5)' },
} satisfies ChartConfig

export const categoryChartConfig = {
  failed: { label: 'Failed', color: 'var(--chart-2)' },
  broken: { label: 'Broken', color: 'var(--chart-3)' },
} satisfies ChartConfig

export const sparklineChartConfig = {
  passRate: { label: 'Pass Rate', color: 'var(--chart-5)' },
} satisfies ChartConfig

// Keep STATUS_COLORS for non-chart usage (TimelineChart, CategoryBreakdown summary dots)
export const STATUS_COLORS = {
  passed: '#16a34a',  // green-600
  failed: '#dc2626',  // red-600
  broken: '#d97706',  // amber-600
  skipped: '#6b7280', // gray-500
} as const

export interface StatusTrendPoint {
  name: string
  passed: number
  failed: number
  broken: number
  skipped: number
}

export interface PassRateTrendPoint {
  name: string
  passRate: number
}

export interface DurationTrendPoint {
  name: string
  durationSec: number
}

export interface StatusPiePoint {
  name: string
  value: number
  color: string
}

function sorted(entries: ReportHistoryEntry[]): ReportHistoryEntry[] {
  return [...entries].reverse()
}

export function toStatusTrendData(entries: ReportHistoryEntry[]): StatusTrendPoint[] {
  return sorted(entries)
    .filter((e) => e.statistic !== null)
    .map((e) => ({
      name: `#${e.report_id}`,
      passed: e.statistic!.passed,
      failed: e.statistic!.failed,
      broken: e.statistic!.broken,
      skipped: e.statistic!.skipped,
    }))
}

export function toPassRateTrendData(entries: ReportHistoryEntry[]): PassRateTrendPoint[] {
  return sorted(entries)
    .filter((e) => e.statistic !== null)
    .map((e) => ({
      name: `#${e.report_id}`,
      passRate: calcPassRate(e.statistic!.passed, e.statistic!.total),
    }))
}

export function toDurationTrendData(entries: ReportHistoryEntry[]): DurationTrendPoint[] {
  return sorted(entries)
    .filter((e) => e.duration_ms !== null)
    .map((e) => ({
      name: `#${e.report_id}`,
      durationSec: Math.round(e.duration_ms! / 1000),
    }))
}

export function toStatusPieData(entries: ReportHistoryEntry[]): StatusPiePoint[] {
  if (entries.length === 0) return []
  const latest = entries[0]
  if (!latest.statistic) return []
  const { passed, failed, broken, skipped } = latest.statistic
  return [
    { name: 'Passed', value: passed, color: STATUS_COLORS.passed },
    { name: 'Failed', value: failed, color: STATUS_COLORS.failed },
    { name: 'Broken', value: broken, color: STATUS_COLORS.broken },
    { name: 'Skipped', value: skipped, color: STATUS_COLORS.skipped },
  ].filter((d) => d.value > 0)
}

// ---------------------------------------------------------------------------
// Category breakdown utilities (A4)
// ---------------------------------------------------------------------------

export interface CategoryBreakdownPoint {
  name: string
  failed: number
  broken: number
  total: number
  color: string
}

export const CATEGORY_COLORS: Record<string, string> = {
  'Product defects': '#dc2626', // red-600
  'Test defects': '#d97706',    // amber-600
} as const

const CATEGORY_DEFAULT_COLOR = '#6b7280' // gray-500

export function toCategoryBreakdownData(entries: CategoryEntry[]): CategoryBreakdownPoint[] {
  return entries
    .filter((e) => e.matchedStatistic && e.matchedStatistic.total > 0)
    .map((e) => ({
      name: e.name,
      failed: e.matchedStatistic!.failed,
      broken: e.matchedStatistic!.broken,
      total: e.matchedStatistic!.total,
      color: CATEGORY_COLORS[e.name] ?? CATEGORY_DEFAULT_COLOR,
    }))
}

// ---------------------------------------------------------------------------
// Timeline lane utilities (G3)
// ---------------------------------------------------------------------------

export interface TimelineLane {
  id: string
  label: string
}

export type LaneStrategy = 'thread' | 'host' | 'default'

export function detectLaneStrategy(testCases: TimelineTestCase[]): LaneStrategy {
  if (testCases.some((tc) => tc.thread)) return 'thread'
  if (testCases.some((tc) => tc.host)) return 'host'
  return 'default'
}

export function toTimelineLanes(testCases: TimelineTestCase[], strategy: LaneStrategy): TimelineLane[] {
  if (strategy === 'default') return [{ id: 'default', label: 'Tests' }]
  const seen = new Set<string>()
  const lanes: TimelineLane[] = []
  for (const tc of testCases) {
    const key = strategy === 'thread' ? tc.thread : tc.host
    if (key && !seen.has(key)) {
      seen.add(key)
      lanes.push({ id: key, label: key })
    }
  }
  if (lanes.length === 0) return [{ id: 'default', label: 'Tests' }]
  return lanes
}
