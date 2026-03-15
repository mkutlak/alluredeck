import type { TimelineTestCase } from '@/types/api'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface GanttBar {
  tc: TimelineTestCase
  /** Pixel x position from the left edge. */
  x: number
  /** Pixel width; minimum 2px so tiny tests stay visible. */
  width: number
  /** Pixel y position: row * (barHeight + barGap). */
  y: number
  /** Zero-based row index. */
  row: number
}

export interface GanttLayout {
  bars: GanttBar[]
  totalHeight: number
  rowCount: number
}

export interface MinimapBar {
  tc: TimelineTestCase
  x: number
  width: number
  y: number
}

// ---------------------------------------------------------------------------
// computeGanttLayout
// ---------------------------------------------------------------------------

/**
 * Computes pixel-positioned Gantt bars with greedy row-stacking so that no
 * two bars in the same row overlap.
 *
 * @param testCases  Source test cases (not mutated).
 * @param xScale     Maps an absolute ms timestamp to a pixel x value.
 * @param barHeight  Height of each bar in pixels.
 * @param barGap     Vertical gap between rows in pixels.
 */
export function computeGanttLayout(
  testCases: TimelineTestCase[],
  xScale: (ms: number) => number,
  barHeight: number,
  barGap: number,
): GanttLayout {
  if (testCases.length === 0) {
    return { bars: [], totalHeight: 0, rowCount: 0 }
  }

  const sorted = [...testCases].sort((a, b) => a.start - b.start)

  const bars: GanttBar[] = []
  /** Pixel right-edge of the last bar placed in each row. */
  const rowEnds: number[] = []

  for (const tc of sorted) {
    const x = xScale(tc.start)
    const width = Math.max(2, xScale(tc.stop) - xScale(tc.start))

    let row = -1
    for (let i = 0; i < rowEnds.length; i++) {
      if (x >= rowEnds[i]) {
        row = i
        break
      }
    }
    if (row === -1) {
      row = rowEnds.length
      rowEnds.push(0)
    }
    rowEnds[row] = x + width

    bars.push({ tc, x, width, y: row * (barHeight + barGap), row })
  }

  const rowCount = rowEnds.length
  return { bars, totalHeight: rowCount * (barHeight + barGap), rowCount }
}

// ---------------------------------------------------------------------------
// filterByTimeRange
// ---------------------------------------------------------------------------

/**
 * Returns test cases that overlap the half-open interval [t0, t1).
 * A test overlaps when `tc.stop > t0 && tc.start < t1`.
 */
export function filterByTimeRange(
  testCases: TimelineTestCase[],
  t0: number,
  t1: number,
): TimelineTestCase[] {
  return testCases.filter((tc) => tc.stop > t0 && tc.start < t1)
}

// ---------------------------------------------------------------------------
// computeMinimapBars
// ---------------------------------------------------------------------------

/**
 * Computes minimap bar positions by distributing test cases evenly across a
 * fixed pixel height.
 *
 * @param testCases  Source test cases (not mutated).
 * @param xScale     Maps an absolute ms timestamp to a pixel x value.
 * @param height     Total pixel height of the minimap.
 */
export function computeMinimapBars(
  testCases: TimelineTestCase[],
  xScale: (ms: number) => number,
  height: number,
): MinimapBar[] {
  if (testCases.length === 0) return []

  const sorted = [...testCases].sort((a, b) => a.start - b.start)
  const total = sorted.length

  return sorted.map((tc, index) => ({
    tc,
    x: xScale(tc.start),
    width: Math.max(1, xScale(tc.stop) - xScale(tc.start)),
    y: (index / total) * height,
  }))
}
