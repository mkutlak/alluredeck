import type { CategoryEntry, ReportHistoryEntry } from '@/types/api'
import type { ChartConfig } from '@/components/ui/chart'
import { calcPassRate } from './utils'

// ---------------------------------------------------------------------------
// KPI Summary types
// ---------------------------------------------------------------------------

export interface KpiData {
  passRate: number
  passRateTrend: number[]
  totalTests: number
  totalTestsTrend: number[]
  avgDuration: number
  durationTrend: number[]
  failedCount: number
  failedTrend: number[]
}

// ChartConfig objects for each chart type — use CSS variables for theming
export const statusChartConfig = {
  passed: { label: 'Passed', color: 'hsl(var(--chart-1))' },
  failed: { label: 'Failed', color: 'hsl(var(--chart-2))' },
  broken: { label: 'Broken', color: 'hsl(var(--chart-3))' },
  skipped: { label: 'Skipped', color: 'hsl(var(--chart-4))' },
} satisfies ChartConfig

export const passRateChartConfig = {
  passRate: { label: 'Pass Rate', color: 'hsl(var(--chart-5))' },
} satisfies ChartConfig

export const durationChartConfig = {
  durationSec: { label: 'Duration', color: 'hsl(var(--chart-5))' },
} satisfies ChartConfig

export const categoryChartConfig = {
  failed: { label: 'Failed', color: 'hsl(var(--chart-2))' },
  broken: { label: 'Broken', color: 'hsl(var(--chart-3))' },
} satisfies ChartConfig

export const sparklineChartConfig = {
  passRate: { label: 'Pass Rate', color: 'hsl(var(--chart-5))' },
} satisfies ChartConfig

import { STATUS_COLORS, CATEGORY_COLORS, CATEGORY_DEFAULT_COLOR } from '@/lib/status-colors'
export { STATUS_COLORS, CATEGORY_COLORS, CATEGORY_DEFAULT_COLOR }

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

export interface AllTrendData {
  status: StatusTrendPoint[]
  passRate: PassRateTrendPoint[]
  duration: DurationTrendPoint[]
}

export function toAllTrendData(entries: ReportHistoryEntry[]): AllTrendData {
  const reversed = [...entries].reverse()
  const status: StatusTrendPoint[] = []
  const passRate: PassRateTrendPoint[] = []
  const duration: DurationTrendPoint[] = []

  for (const e of reversed) {
    const name = `#${e.report_id}`
    if (e.statistic !== null) {
      status.push({
        name,
        passed: e.statistic!.passed,
        failed: e.statistic!.failed,
        broken: e.statistic!.broken,
        skipped: e.statistic!.skipped,
      })
      passRate.push({ name, passRate: calcPassRate(e.statistic!.passed, e.statistic!.total) })
    }
    if (e.duration_ms !== null) {
      duration.push({ name, durationSec: Math.round(e.duration_ms! / 1000) })
    }
  }

  return { status, passRate, duration }
}

// ---------------------------------------------------------------------------
// KPI Summary utilities
// ---------------------------------------------------------------------------

export function toKpiData(entries: ReportHistoryEntry[]): KpiData | null {
  if (entries.length === 0) return null

  const latest = entries[0]
  if (!latest.statistic) return null

  // Take last 10 reports, reverse to chronological for sparklines
  const sparklineEntries = entries.slice(0, 10).reverse()

  const passRateTrend = sparklineEntries
    .filter((e) => e.statistic)
    .map((e) => calcPassRate(e.statistic!.passed, e.statistic!.total))

  const totalTestsTrend = sparklineEntries
    .filter((e) => e.statistic)
    .map((e) => e.statistic!.total)

  const durationTrend = sparklineEntries
    .filter((e) => e.duration_ms != null)
    .map((e) => e.duration_ms!)

  const failedTrend = sparklineEntries
    .filter((e) => e.statistic)
    .map((e) => e.statistic!.failed + e.statistic!.broken)

  return {
    passRate: calcPassRate(latest.statistic.passed, latest.statistic.total),
    passRateTrend,
    totalTests: latest.statistic.total,
    totalTestsTrend,
    avgDuration: latest.duration_ms ?? 0,
    durationTrend,
    failedCount: latest.statistic.failed + latest.statistic.broken,
    failedTrend,
  }
}
