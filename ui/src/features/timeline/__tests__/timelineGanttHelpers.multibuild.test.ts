import { describe, it, expect } from 'vitest'
import type { TimelineTestCase, TimelineBuildEntry } from '@/types/api'
import { computeMultiBuildLayout } from '../timelineGanttHelpers'

function makeTC(overrides: Partial<TimelineTestCase> = {}): TimelineTestCase {
  return {
    name: 'test',
    full_name: 'suite.test',
    status: 'passed',
    start: 0,
    stop: 1000,
    duration: 1000,
    thread: '',
    host: '',
    ...overrides,
  }
}

function xScale(ms: number): number {
  return ms / 10
}

function makeBuild(
  buildOrder: number,
  testCases: TimelineTestCase[],
  createdAt = '2026-03-25T12:00:00Z',
): TimelineBuildEntry {
  return {
    build_order: buildOrder,
    created_at: createdAt,
    test_cases: testCases,
    summary: {
      total: testCases.length,
      min_start: Math.min(...testCases.map((tc) => tc.start)),
      max_stop: Math.max(...testCases.map((tc) => tc.stop)),
      total_duration: testCases.reduce((sum, tc) => sum + tc.duration, 0),
      truncated: false,
    },
  }
}

describe('computeMultiBuildLayout', () => {
  const barHeight = 6
  const barGap = 2
  const bandGap = 24

  it('returns empty bands and 0 height for empty builds array', () => {
    const result = computeMultiBuildLayout([], xScale, barHeight, barGap, bandGap)
    expect(result.bands).toEqual([])
    expect(result.totalHeight).toBe(0)
  })

  it('single build produces one band at yOffset 0', () => {
    const tc = makeTC({ start: 0, stop: 1000 })
    const builds = [makeBuild(1, [tc])]
    const result = computeMultiBuildLayout(builds, xScale, barHeight, barGap, bandGap)

    expect(result.bands).toHaveLength(1)
    expect(result.bands[0].buildOrder).toBe(1)
    expect(result.bands[0].yOffset).toBe(0)
    expect(result.bands[0].bandHeight).toBeGreaterThan(0)
    expect(result.totalHeight).toBe(result.bands[0].bandHeight)
  })

  it('three builds produce non-overlapping bands', () => {
    const tc1 = makeTC({ start: 0, stop: 2000 })
    const tc2 = makeTC({ start: 500, stop: 1500 })
    const builds = [
      makeBuild(3, [tc1, tc2]),
      makeBuild(2, [tc1]),
      makeBuild(1, [tc1, tc2]),
    ]
    const result = computeMultiBuildLayout(builds, xScale, barHeight, barGap, bandGap)

    expect(result.bands).toHaveLength(3)

    // Each band's yOffset should be after the previous band's yOffset + bandHeight + bandGap
    for (let i = 1; i < result.bands.length; i++) {
      const prev = result.bands[i - 1]
      expect(result.bands[i].yOffset).toBe(prev.yOffset + prev.bandHeight + bandGap)
    }
  })

  it('ten builds all get unique non-overlapping offsets', () => {
    const tc = makeTC({ start: 0, stop: 1000 })
    const builds = Array.from({ length: 10 }, (_, i) => makeBuild(10 - i, [tc]))
    const result = computeMultiBuildLayout(builds, xScale, barHeight, barGap, bandGap)

    expect(result.bands).toHaveLength(10)

    // Verify no overlapping bands
    for (let i = 1; i < result.bands.length; i++) {
      const prevEnd = result.bands[i - 1].yOffset + result.bands[i - 1].bandHeight
      expect(result.bands[i].yOffset).toBeGreaterThanOrEqual(prevEnd + bandGap)
    }

    // Total height should equal last band's yOffset + its bandHeight
    const lastBand = result.bands[result.bands.length - 1]
    expect(result.totalHeight).toBe(lastBand.yOffset + lastBand.bandHeight)
  })

  it('preserves build order and createdAt in band metadata', () => {
    const tc = makeTC({ start: 0, stop: 1000 })
    const builds = [
      makeBuild(44, [tc], '2026-03-25T00:00:00Z'),
      makeBuild(43, [tc], '2026-03-24T00:00:00Z'),
    ]
    const result = computeMultiBuildLayout(builds, xScale, barHeight, barGap, bandGap)

    expect(result.bands[0].buildOrder).toBe(44)
    expect(result.bands[0].createdAt).toBe('2026-03-25T00:00:00Z')
    expect(result.bands[1].buildOrder).toBe(43)
    expect(result.bands[1].createdAt).toBe('2026-03-24T00:00:00Z')
  })

  it('each band contains a valid GanttLayout from computeGanttLayout', () => {
    const tc1 = makeTC({ start: 0, stop: 2000 })
    const tc2 = makeTC({ start: 500, stop: 1500 })
    const builds = [makeBuild(1, [tc1, tc2])]
    const result = computeMultiBuildLayout(builds, xScale, barHeight, barGap, bandGap)

    const band = result.bands[0]
    expect(band.layout.bars).toHaveLength(2)
    expect(band.layout.rowCount).toBeGreaterThan(0)
    expect(band.layout.totalHeight).toBeGreaterThan(0)
    expect(band.bandHeight).toBe(band.layout.totalHeight)
  })

  it('build with no test cases has 0 band height', () => {
    const builds = [makeBuild(1, [])]
    // Fix makeBuild for empty arrays
    builds[0].summary.min_start = 0
    builds[0].summary.max_stop = 0
    const result = computeMultiBuildLayout(builds, xScale, barHeight, barGap, bandGap)

    expect(result.bands).toHaveLength(1)
    expect(result.bands[0].bandHeight).toBe(0)
  })
})
