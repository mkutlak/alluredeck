import { describe, it, expect } from 'vitest'
import type { TimelineTestCase } from '@/types/api'
import { computeGanttLayout, filterByTimeRange, computeMinimapBars } from '../timelineGanttHelpers'

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

/** Simple linear xScale: maps ms timestamp to pixel value. */
function xScale(ms: number): number {
  return ms / 10 // 1px per 10ms
}

// ---------------------------------------------------------------------------
// computeGanttLayout
// ---------------------------------------------------------------------------
describe('computeGanttLayout', () => {
  it('empty array returns empty bars and 0 height', () => {
    const layout = computeGanttLayout([], xScale, 20, 4)
    expect(layout.bars).toEqual([])
    expect(layout.totalHeight).toBe(0)
    expect(layout.rowCount).toBe(0)
  })

  it('single test case produces one bar at correct position', () => {
    const tc = makeTC({ start: 100, stop: 300 })
    const layout = computeGanttLayout([tc], xScale, 20, 4)
    expect(layout.bars).toHaveLength(1)
    const bar = layout.bars[0]
    expect(bar.tc).toBe(tc)
    expect(bar.x).toBe(10) // 100 / 10
    expect(bar.width).toBe(20) // (300 - 100) / 10
    expect(bar.row).toBe(0)
    expect(bar.y).toBe(0) // row 0 * (20 + 4)
  })

  it('two non-overlapping tests stack in same row', () => {
    const tc1 = makeTC({ start: 0, stop: 1000 })
    const tc2 = makeTC({ start: 2000, stop: 3000 })
    const layout = computeGanttLayout([tc1, tc2], xScale, 20, 4)
    expect(layout.rowCount).toBe(1)
    expect(layout.bars[0].row).toBe(0)
    expect(layout.bars[1].row).toBe(0)
  })

  it('two overlapping tests go in different rows', () => {
    const tc1 = makeTC({ start: 0, stop: 2000 })
    const tc2 = makeTC({ start: 500, stop: 1500 })
    const layout = computeGanttLayout([tc1, tc2], xScale, 20, 4)
    expect(layout.rowCount).toBe(2)
    expect(layout.bars[0].row).toBe(0)
    expect(layout.bars[1].row).toBe(1)
  })

  it('tests are sorted by start time before layout', () => {
    const tc1 = makeTC({ name: 'late', start: 5000, stop: 6000 })
    const tc2 = makeTC({ name: 'early', start: 0, stop: 1000 })
    const layout = computeGanttLayout([tc1, tc2], xScale, 20, 4)
    // After sorting, tc2 (early) is processed first and goes to row 0
    expect(layout.bars[0].tc.name).toBe('early')
    expect(layout.bars[1].tc.name).toBe('late')
    expect(layout.rowCount).toBe(1)
  })

  it('bar height and gap are respected in y-positions', () => {
    const barHeight = 16
    const barGap = 6
    const tc1 = makeTC({ start: 0, stop: 2000 })
    const tc2 = makeTC({ start: 500, stop: 1500 })
    const layout = computeGanttLayout([tc1, tc2], xScale, barHeight, barGap)
    expect(layout.bars[0].y).toBe(0) // row 0: 0 * (16 + 6)
    expect(layout.bars[1].y).toBe(22) // row 1: 1 * (16 + 6)
  })

  it('returns correct totalHeight and rowCount', () => {
    const barHeight = 20
    const barGap = 4
    const tc1 = makeTC({ start: 0, stop: 2000 })
    const tc2 = makeTC({ start: 500, stop: 1500 })
    const tc3 = makeTC({ start: 800, stop: 1200 })
    const layout = computeGanttLayout([tc1, tc2, tc3], xScale, barHeight, barGap)
    expect(layout.rowCount).toBe(3)
    expect(layout.totalHeight).toBe(3 * (barHeight + barGap))
  })

  it('enforces minimum bar width of 2px for very short tests', () => {
    const tc = makeTC({ start: 0, stop: 1 }) // 1ms → 0.1px without clamp
    const layout = computeGanttLayout([tc], xScale, 20, 4)
    expect(layout.bars[0].width).toBe(2)
  })

  it('does not mutate the input array', () => {
    const tc1 = makeTC({ start: 5000, stop: 6000 })
    const tc2 = makeTC({ start: 0, stop: 1000 })
    const input = [tc1, tc2]
    computeGanttLayout(input, xScale, 20, 4)
    expect(input[0]).toBe(tc1) // original order preserved
    expect(input[1]).toBe(tc2)
  })
})

// ---------------------------------------------------------------------------
// filterByTimeRange
// ---------------------------------------------------------------------------
describe('filterByTimeRange', () => {
  it('empty array returns empty', () => {
    expect(filterByTimeRange([], 0, 1000)).toEqual([])
  })

  it('test fully inside range is included', () => {
    const tc = makeTC({ start: 200, stop: 800 })
    expect(filterByTimeRange([tc], 0, 1000)).toContain(tc)
  })

  it('test fully outside range is excluded — before range', () => {
    const tc = makeTC({ start: 0, stop: 100 })
    expect(filterByTimeRange([tc], 200, 1000)).toEqual([])
  })

  it('test fully outside range is excluded — after range', () => {
    const tc = makeTC({ start: 1100, stop: 2000 })
    expect(filterByTimeRange([tc], 0, 1000)).toEqual([])
  })

  it('test partially overlapping start is included', () => {
    const tc = makeTC({ start: 500, stop: 1500 })
    expect(filterByTimeRange([tc], 1000, 2000)).toContain(tc)
  })

  it('test partially overlapping end is included', () => {
    const tc = makeTC({ start: 0, stop: 500 })
    expect(filterByTimeRange([tc], -100, 100)).toContain(tc)
  })

  it('test spanning the entire range is included', () => {
    const tc = makeTC({ start: 0, stop: 10000 })
    expect(filterByTimeRange([tc], 1000, 5000)).toContain(tc)
  })

  it('test that ends exactly at t0 is excluded (no overlap)', () => {
    const tc = makeTC({ start: 0, stop: 1000 })
    expect(filterByTimeRange([tc], 1000, 2000)).toEqual([])
  })

  it('test that starts exactly at t1 is excluded (no overlap)', () => {
    const tc = makeTC({ start: 2000, stop: 3000 })
    expect(filterByTimeRange([tc], 0, 2000)).toEqual([])
  })
})

// ---------------------------------------------------------------------------
// computeMinimapBars
// ---------------------------------------------------------------------------
describe('computeMinimapBars', () => {
  it('empty array returns empty', () => {
    expect(computeMinimapBars([], xScale, 100)).toEqual([])
  })

  it('single bar has y of 0 (index 0 / total 1 * height)', () => {
    const tc = makeTC({ start: 0, stop: 1000 })
    const bars = computeMinimapBars([tc], xScale, 100)
    expect(bars).toHaveLength(1)
    expect(bars[0].y).toBe(0)
    expect(bars[0].tc).toBe(tc)
  })

  it('bars are distributed across the height', () => {
    const tcs = [
      makeTC({ name: 'a', start: 0, stop: 1000 }),
      makeTC({ name: 'b', start: 1000, stop: 2000 }),
      makeTC({ name: 'c', start: 2000, stop: 3000 }),
    ]
    const height = 100
    const bars = computeMinimapBars(tcs, xScale, height)
    expect(bars[0].y).toBe(0) // (0 / 3) * 100
    expect(bars[1].y).toBeCloseTo((1 / 3) * 100) // ~33.33
    expect(bars[2].y).toBeCloseTo((2 / 3) * 100) // ~66.67
  })

  it('each bar has correct x position from xScale', () => {
    const tc = makeTC({ start: 500, stop: 1500 })
    const bars = computeMinimapBars([tc], xScale, 100)
    expect(bars[0].x).toBe(50) // xScale(500) = 50
  })

  it('each bar has correct width from xScale', () => {
    const tc = makeTC({ start: 0, stop: 2000 })
    const bars = computeMinimapBars([tc], xScale, 100)
    expect(bars[0].width).toBe(200) // xScale(2000) - xScale(0)
  })

  it('enforces minimum width of 1px', () => {
    const tc = makeTC({ start: 0, stop: 1 }) // 1ms → 0.1px
    const bars = computeMinimapBars([tc], xScale, 100)
    expect(bars[0].width).toBe(1)
  })

  it('bars are sorted by start time (sorted copy, not mutated)', () => {
    const tc1 = makeTC({ name: 'late', start: 2000, stop: 3000 })
    const tc2 = makeTC({ name: 'early', start: 0, stop: 1000 })
    const input = [tc1, tc2]
    const bars = computeMinimapBars(input, xScale, 100)
    expect(bars[0].tc.name).toBe('early')
    expect(bars[1].tc.name).toBe('late')
    expect(input[0]).toBe(tc1) // original not mutated
  })
})
