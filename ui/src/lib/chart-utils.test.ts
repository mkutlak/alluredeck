import { describe, it, expect } from 'vitest'
import {
  toStatusPieData,
  toCategoryBreakdownData,
  toAllTrendData,
  STATUS_COLORS,
  CATEGORY_COLORS,
} from './chart-utils'
import type { CategoryEntry, ReportHistoryEntry } from '@/types/api'

const makeEntry = (
  id: string,
  overrides: Partial<ReportHistoryEntry> = {},
): ReportHistoryEntry => ({
  report_id: id,
  is_latest: false,
  generated_at: `2024-01-${id.padStart(2, '0')}T10:00:00Z`,
  duration_ms: 5000,
  statistic: {
    passed: 8,
    failed: 1,
    broken: 0,
    skipped: 1,
    unknown: 0,
    total: 10,
  },
  ...overrides,
})

describe('STATUS_COLORS', () => {
  it('has entries for all 4 statuses', () => {
    expect(STATUS_COLORS.passed).toBeDefined()
    expect(STATUS_COLORS.failed).toBeDefined()
    expect(STATUS_COLORS.broken).toBeDefined()
    expect(STATUS_COLORS.skipped).toBeDefined()
  })
})


describe('toCategoryBreakdownData', () => {
  const makeCategory = (
    name: string,
    matchedStatistic: CategoryEntry['matchedStatistic'] = {
      failed: 2,
      broken: 1,
      known: 0,
      unknown: 0,
      total: 3,
    },
  ): CategoryEntry => ({ name, matchedStatistic })

  it('returns empty array for empty input', () => {
    expect(toCategoryBreakdownData([])).toEqual([])
  })

  it('filters out categories with null matchedStatistic', () => {
    const entries = [makeCategory('Product defects'), makeCategory('Test defects', null)]
    const result = toCategoryBreakdownData(entries)
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('Product defects')
  })

  it('filters out categories with zero total', () => {
    const entries = [
      makeCategory('Product defects'),
      makeCategory('Empty', { failed: 0, broken: 0, known: 0, unknown: 0, total: 0 }),
    ]
    const result = toCategoryBreakdownData(entries)
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('Product defects')
  })

  it('maps fields correctly for known category', () => {
    const entries = [
      makeCategory('Product defects', { failed: 2, broken: 1, known: 0, unknown: 0, total: 3 }),
    ]
    const result = toCategoryBreakdownData(entries)
    expect(result[0].name).toBe('Product defects')
    expect(result[0].failed).toBe(2)
    expect(result[0].broken).toBe(1)
    expect(result[0].total).toBe(3)
    expect(result[0].color).toBe(CATEGORY_COLORS['Product defects'])
  })

  it('uses default color for unknown category names', () => {
    const entries = [makeCategory('Some other defect')]
    const result = toCategoryBreakdownData(entries)
    expect(result[0].color).toBe('#8c8fa1')
  })
})

describe('toStatusPieData', () => {
  it('returns empty array for empty input', () => {
    expect(toStatusPieData([])).toEqual([])
  })

  it('uses first entry (latest) for pie data', () => {
    const entries = [
      makeEntry('3', {
        statistic: { passed: 9, failed: 0, broken: 0, skipped: 1, unknown: 0, total: 10 },
      }),
      makeEntry('2', {
        statistic: { passed: 5, failed: 5, broken: 0, skipped: 0, unknown: 0, total: 10 },
      }),
    ]
    const result = toStatusPieData(entries)
    const passed = result.find((d) => d.name === 'Passed')
    expect(passed?.value).toBe(9)
  })

  it('filters out zero-value statuses', () => {
    const entries = [
      makeEntry('1', {
        statistic: { passed: 8, failed: 0, broken: 0, skipped: 2, unknown: 0, total: 10 },
      }),
    ]
    const result = toStatusPieData(entries)
    expect(result.every((d) => d.value > 0)).toBe(true)
    expect(result.length).toBe(2)
  })
})

describe('toAllTrendData', () => {
  it('returns empty arrays for empty input', () => {
    const result = toAllTrendData([])
    expect(result.status).toEqual([])
    expect(result.passRate).toEqual([])
    expect(result.duration).toEqual([])
  })

  it('filters nulls independently (stat null but duration present)', () => {
    const entries = [makeEntry('1', { statistic: null, duration_ms: 5000 })]
    const result = toAllTrendData(entries)
    expect(result.status).toHaveLength(0)
    expect(result.passRate).toHaveLength(0)
    expect(result.duration).toHaveLength(1)
  })
})
