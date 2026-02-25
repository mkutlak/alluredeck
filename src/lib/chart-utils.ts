import type { ReportHistoryEntry } from '@/types/api'
import { calcPassRate } from './utils'

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
