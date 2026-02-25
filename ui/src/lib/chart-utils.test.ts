import { describe, it, expect } from 'vitest'
import {
  toStatusTrendData,
  toPassRateTrendData,
  toDurationTrendData,
  toStatusPieData,
  STATUS_COLORS,
} from './chart-utils'
import type { ReportHistoryEntry } from '@/types/api'

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

describe('toStatusTrendData', () => {
  it('returns empty array for empty input', () => {
    expect(toStatusTrendData([])).toEqual([])
  })

  it('filters entries with null statistic', () => {
    const entries = [makeEntry('1'), makeEntry('2', { statistic: null })]
    const result = toStatusTrendData(entries)
    expect(result).toHaveLength(1)
  })

  it('reverses order (oldest first)', () => {
    const entries = [makeEntry('3'), makeEntry('2'), makeEntry('1')]
    const result = toStatusTrendData(entries)
    expect(result[0].name).toBe('#1')
    expect(result[2].name).toBe('#3')
  })

  it('maps statistic fields correctly', () => {
    const entries = [makeEntry('1', { statistic: { passed: 5, failed: 2, broken: 1, skipped: 2, unknown: 0, total: 10 } })]
    const result = toStatusTrendData(entries)
    expect(result[0].passed).toBe(5)
    expect(result[0].failed).toBe(2)
    expect(result[0].broken).toBe(1)
    expect(result[0].skipped).toBe(2)
  })
})

describe('toPassRateTrendData', () => {
  it('returns empty array for empty input', () => {
    expect(toPassRateTrendData([])).toEqual([])
  })

  it('calculates pass rate correctly', () => {
    const entries = [makeEntry('1', { statistic: { passed: 9, failed: 1, broken: 0, skipped: 0, unknown: 0, total: 10 } })]
    const result = toPassRateTrendData(entries)
    expect(result[0].passRate).toBe(90)
  })

  it('handles zero total without throwing', () => {
    const entries = [makeEntry('1', { statistic: { passed: 0, failed: 0, broken: 0, skipped: 0, unknown: 0, total: 0 } })]
    const result = toPassRateTrendData(entries)
    expect(result[0].passRate).toBe(0)
  })
})

describe('toDurationTrendData', () => {
  it('returns empty array for empty input', () => {
    expect(toDurationTrendData([])).toEqual([])
  })

  it('filters entries with null duration', () => {
    const entries = [makeEntry('1'), makeEntry('2', { duration_ms: null })]
    const result = toDurationTrendData(entries)
    expect(result).toHaveLength(1)
  })

  it('converts ms to seconds', () => {
    const entries = [makeEntry('1', { duration_ms: 65000 })]
    const result = toDurationTrendData(entries)
    expect(result[0].durationSec).toBe(65)
  })
})

describe('toStatusPieData', () => {
  it('returns empty array for empty input', () => {
    expect(toStatusPieData([])).toEqual([])
  })

  it('uses first entry (latest) for pie data', () => {
    const entries = [
      makeEntry('3', { statistic: { passed: 9, failed: 0, broken: 0, skipped: 1, unknown: 0, total: 10 } }),
      makeEntry('2', { statistic: { passed: 5, failed: 5, broken: 0, skipped: 0, unknown: 0, total: 10 } }),
    ]
    const result = toStatusPieData(entries)
    const passed = result.find((d) => d.name === 'Passed')
    expect(passed?.value).toBe(9)
  })

  it('filters out zero-value statuses', () => {
    const entries = [
      makeEntry('1', { statistic: { passed: 8, failed: 0, broken: 0, skipped: 2, unknown: 0, total: 10 } }),
    ]
    const result = toStatusPieData(entries)
    expect(result.every((d) => d.value > 0)).toBe(true)
    expect(result.length).toBe(2)
  })
})
