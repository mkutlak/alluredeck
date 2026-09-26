import { describe, it, expect } from 'vitest'
import {
  toStatusPieData,
  toCategoryBreakdownData,
  STATUS_COLORS,
  CATEGORY_COLORS,
  CATEGORY_DEFAULT_COLOR,
} from './chart-utils'
import type { CategoryEntry, ReportHistoryEntry } from '@/types/api'

const stat = (passed: number, failed: number, broken: number, skipped: number) => ({
  passed,
  failed,
  broken,
  skipped,
  unknown: 0,
  total: passed + failed + broken + skipped,
})

describe('toCategoryBreakdownData', () => {
  it('keeps categories with matches, mapping known names to their colour', () => {
    const matched = { failed: 2, broken: 1, known: 0, unknown: 0, total: 3 }
    const entries: CategoryEntry[] = [
      { name: 'Product defects', matchedStatistic: matched },
      { name: 'Test defects', matchedStatistic: null },
      { name: 'Empty', matchedStatistic: { ...matched, failed: 0, broken: 0, total: 0 } },
      { name: 'Some other defect', matchedStatistic: matched },
    ]
    expect(toCategoryBreakdownData(entries)).toEqual([
      {
        name: 'Product defects',
        failed: 2,
        broken: 1,
        total: 3,
        color: CATEGORY_COLORS['Product defects'],
      },
      { name: 'Some other defect', failed: 2, broken: 1, total: 3, color: CATEGORY_DEFAULT_COLOR },
    ])
  })
})

describe('toStatusPieData', () => {
  it('returns empty array for empty input', () => {
    expect(toStatusPieData([])).toEqual([])
  })

  it('slices the latest (first) entry and drops zero-value statuses', () => {
    const entry = (report_id: string, statistic: ReportHistoryEntry['statistic']) => ({
      report_id,
      is_latest: false,
      generated_at: '2024-01-01T10:00:00Z',
      duration_ms: 5000,
      statistic,
    })
    expect(toStatusPieData([entry('3', stat(9, 0, 0, 1)), entry('2', stat(5, 5, 0, 0))])).toEqual([
      { name: 'Passed', value: 9, color: STATUS_COLORS.passed },
      { name: 'Skipped', value: 1, color: STATUS_COLORS.skipped },
    ])
  })
})
