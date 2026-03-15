import { describe, it, expect } from 'vitest'
import { toKpiData } from '../chart-utils'
import type { ReportHistoryEntry } from '@/types/api'

// Helper to create a minimal ReportHistoryEntry
function makeEntry(overrides: Partial<ReportHistoryEntry> = {}): ReportHistoryEntry {
  return {
    report_id: '1',
    is_latest: false,
    generated_at: '2024-01-01T00:00:00Z',
    duration_ms: 5000,
    statistic: { passed: 90, failed: 5, broken: 3, skipped: 2, unknown: 0, total: 100 },
    ...overrides,
  }
}

describe('toKpiData', () => {
  it('returns null for empty entries', () => {
    expect(toKpiData([])).toBeNull()
  })

  it('returns null when latest has no statistic', () => {
    expect(toKpiData([makeEntry({ statistic: null })])).toBeNull()
  })

  it('extracts KPI values from latest entry', () => {
    const result = toKpiData([makeEntry()])
    expect(result).not.toBeNull()
    expect(result!.passRate).toBe(90)
    expect(result!.totalTests).toBe(100)
    expect(result!.avgDuration).toBe(5000)
    expect(result!.failedCount).toBe(8) // 5 failed + 3 broken
  })

  it('generates sparkline trends from last 10 entries', () => {
    // Simulate newest-first: entries[0] is most recent (highest pass rate)
    const entries = Array.from({ length: 15 }, (_, i) =>
      makeEntry({
        report_id: String(15 - i),
        statistic: {
          passed: 80 + (14 - i),
          failed: 5,
          broken: 3,
          skipped: 2,
          unknown: 0,
          total: 90 + (14 - i),
        },
      }),
    )
    const result = toKpiData(entries)
    expect(result!.passRateTrend).toHaveLength(10)
    // Sliced to first 10 (newest), then reversed to chronological:
    // trend[0] = oldest of the 10 (lower pass rate), trend[9] = newest (higher pass rate)
    expect(result!.passRateTrend[0]).toBeLessThan(result!.passRateTrend[9])
  })

  it('returns avgDuration 0 when duration_ms is null', () => {
    const result = toKpiData([makeEntry({ duration_ms: null })])
    expect(result!.avgDuration).toBe(0)
  })
})
