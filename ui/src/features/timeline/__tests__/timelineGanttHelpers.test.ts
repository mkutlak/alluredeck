import { describe, it, expect } from 'vitest'
import type { TimelineTestCase, TimelineBuildEntry } from '@/types/api'
import {
  computeGanttLayout,
  computeMinimapBars,
  computeMultiBuildLayout,
  filterByTimeRange,
} from '../timelineGanttHelpers'

function makeTC(start: number, stop: number): TimelineTestCase {
  return {
    name: `t${start}`,
    full_name: `suite.t${start}`,
    status: 'passed',
    start,
    stop,
    duration: stop - start,
    thread: '',
    host: '',
  }
}

function makeBuild(order: number, testCases: TimelineTestCase[]): TimelineBuildEntry {
  return {
    build_order: order,
    created_at: `2026-03-${order}T00:00:00Z`,
    test_cases: testCases,
    summary: { total: 0, min_start: 0, max_stop: 0, total_duration: 0, truncated: false },
  }
}

/** 1px per 10ms. */
const xScale = (ms: number) => ms / 10

describe('computeGanttLayout', () => {
  it('positions a bar from the x scale', () => {
    const tc = makeTC(100, 300)
    expect(computeGanttLayout([tc], xScale, 20, 4).bars).toEqual([
      { tc, x: 10, width: 20, row: 0, y: 0 },
    ])
  })

  // bars are [start, row, y] in placement order; rows are barHeight 16 + gap 6 = 22px apart.
  it.each([
    { name: 'is empty for no tests', spans: [], bars: [], rows: 0, height: 0 },
    {
      name: 'packs non-overlapping tests into one row',
      spans: [
        [0, 1000],
        [2000, 3000],
      ],
      bars: [
        [0, 0, 0],
        [2000, 0, 0],
      ],
      rows: 1,
      height: 22,
    },
    {
      name: 'stacks overlapping tests into new rows',
      spans: [
        [0, 2000],
        [500, 1500],
        [800, 1200],
      ],
      bars: [
        [0, 0, 0],
        [500, 1, 22],
        [800, 2, 44],
      ],
      rows: 3,
      height: 66,
    },
    {
      name: 'places tests by start time, not input order',
      spans: [
        [5000, 6000],
        [0, 1000],
      ],
      bars: [
        [0, 0, 0],
        [5000, 0, 0],
      ],
      rows: 1,
      height: 22,
    },
  ])('$name', ({ spans, bars, rows, height }) => {
    const input = spans.map(([start, stop]) => makeTC(start!, stop!))
    const before = [...input]
    const layout = computeGanttLayout(input, xScale, 16, 6)

    expect(layout.bars.map((b) => [b.tc.start, b.row, b.y])).toEqual(bars)
    expect(layout.rowCount).toBe(rows)
    expect(layout.totalHeight).toBe(height)
    expect(input).toEqual(before)
  })

  it('keeps very short tests at least 2px wide', () => {
    expect(computeGanttLayout([makeTC(0, 1)], xScale, 20, 4).bars[0]!.width).toBe(2)
  })
})

describe('filterByTimeRange', () => {
  // Overlap with the half-open interval [1000, 2000).
  it.each([
    { name: 'inside', start: 1200, stop: 1800, kept: true },
    { name: 'overlapping the start', start: 500, stop: 1500, kept: true },
    { name: 'overlapping the end', start: 1500, stop: 2500, kept: true },
    { name: 'spanning the range', start: 0, stop: 5000, kept: true },
    { name: 'entirely before', start: 0, stop: 500, kept: false },
    { name: 'entirely after', start: 2500, stop: 3000, kept: false },
    { name: 'ending exactly at t0', start: 0, stop: 1000, kept: false },
    { name: 'starting exactly at t1', start: 2000, stop: 3000, kept: false },
  ])('keeps=$kept a test $name', ({ start, stop, kept }) => {
    const tc = makeTC(start, stop)
    expect(filterByTimeRange([tc], 1000, 2000)).toEqual(kept ? [tc] : [])
  })
})

describe('computeMinimapBars', () => {
  it('orders bars by start, spreads them down the height and keeps them at least 1px wide', () => {
    const input = [makeTC(2000, 3000), makeTC(0, 1), makeTC(500, 1500)]
    const bars = computeMinimapBars(input, xScale, 90)

    expect(bars.map(({ tc, x, width, y }) => [tc.start, x, width, y])).toEqual([
      [0, 0, 1, 0],
      [500, 50, 100, 30],
      [2000, 200, 100, 60],
    ])
    expect(input[0]!.start).toBe(2000)
    expect(computeMinimapBars([], xScale, 90)).toEqual([])
  })
})

describe('computeMultiBuildLayout', () => {
  it('is empty for no builds', () => {
    expect(computeMultiBuildLayout([], xScale, 6, 2, 24)).toEqual({ bands: [], totalHeight: 0 })
  })

  it('stacks one band per build, separated by the band gap', () => {
    const overlapping = [makeTC(0, 2000), makeTC(500, 1500)]
    const { bands, totalHeight } = computeMultiBuildLayout(
      [makeBuild(3, overlapping), makeBuild(2, []), makeBuild(1, [makeTC(0, 1000)])],
      xScale,
      6,
      2,
      24,
    )

    // Band heights: 2 rows * 8px, an empty build 0px, 1 row 8px; 24px between bands.
    expect(
      bands.map((b) => [b.buildOrder, b.createdAt, b.yOffset, b.bandHeight, b.layout.bars.length]),
    ).toEqual([
      [3, '2026-03-3T00:00:00Z', 0, 16, 2],
      [2, '2026-03-2T00:00:00Z', 40, 0, 0],
      [1, '2026-03-1T00:00:00Z', 64, 8, 1],
    ])
    expect(totalHeight).toBe(72)
  })
})
