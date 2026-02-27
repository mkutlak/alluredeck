import { describe, it, expect } from 'vitest'
import type { TimelineTestCase } from '@/types/api'
import { computeTicks, formatRelativeTime, computeBar, stackBarsIntoRows } from '../timelineHelpers'

function makeTestCase(overrides: Partial<TimelineTestCase> = {}): TimelineTestCase {
  return {
    name: 'Test case',
    full_name: 'com.example.TestCase#test',
    status: 'passed',
    start: 1000,
    stop: 3000,
    duration: 2000,
    thread: 'main',
    host: 'host-1',
    ...overrides,
  }
}

// ---------------------------------------------------------------------------
// formatRelativeTime
// ---------------------------------------------------------------------------
describe('formatRelativeTime', () => {
  it('formats 0ms as +0s', () => {
    expect(formatRelativeTime(0)).toBe('+0s')
  })

  it('formats whole seconds', () => {
    expect(formatRelativeTime(5000)).toBe('+5s')
    expect(formatRelativeTime(30000)).toBe('+30s')
    expect(formatRelativeTime(59000)).toBe('+59s')
  })

  it('formats exact minutes without trailing seconds', () => {
    expect(formatRelativeTime(60000)).toBe('+1m')
    expect(formatRelativeTime(120000)).toBe('+2m')
  })

  it('formats minutes with remaining seconds', () => {
    expect(formatRelativeTime(90000)).toBe('+1m 30s')
    expect(formatRelativeTime(150000)).toBe('+2m 30s')
  })

  it('floors sub-second durations to nearest second', () => {
    expect(formatRelativeTime(4500)).toBe('+4s')
    expect(formatRelativeTime(999)).toBe('+0s')
  })
})

// ---------------------------------------------------------------------------
// computeTicks
// ---------------------------------------------------------------------------
describe('computeTicks', () => {
  it('first tick is always at offset 0 with label +0s', () => {
    const ticks = computeTicks(1000000, 1020000)
    expect(ticks[0]).toMatchObject({ ms: 0, label: '+0s' })
  })

  it('returns between 5 and 12 ticks for a normal range', () => {
    const ticks = computeTicks(0, 20000)
    expect(ticks.length).toBeGreaterThanOrEqual(5)
    expect(ticks.length).toBeLessThanOrEqual(12)
  })

  it('tick offsets are non-negative and in ascending order', () => {
    const ticks = computeTicks(0, 30000)
    for (let i = 1; i < ticks.length; i++) {
      expect(ticks[i].ms).toBeGreaterThan(ticks[i - 1].ms)
    }
  })

  it('uses second-scale labels for short ranges', () => {
    const ticks = computeTicks(0, 20000)
    const hasSecondLabel = ticks.some((t) => /^\+\d+s$/.test(t.label))
    expect(hasSecondLabel).toBe(true)
  })

  it('uses minute-scale labels for long ranges', () => {
    const ticks = computeTicks(0, 600000) // 10 minutes
    const hasMinuteLabel = ticks.some((t) => t.label.includes('m'))
    expect(hasMinuteLabel).toBe(true)
  })

  it('handles degenerate range (start === stop) with at least one tick', () => {
    const ticks = computeTicks(5000, 5000)
    expect(ticks.length).toBeGreaterThanOrEqual(1)
    expect(ticks[0]).toMatchObject({ ms: 0, label: '+0s' })
  })
})

// ---------------------------------------------------------------------------
// computeBar
// ---------------------------------------------------------------------------
describe('computeBar', () => {
  it('bar at start has leftPct of 0', () => {
    const tc = makeTestCase({ start: 1000, stop: 3000 })
    const bar = computeBar(tc, 1000, 10000)
    expect(bar.leftPct).toBe(0)
  })

  it('computes correct width percentage', () => {
    const tc = makeTestCase({ start: 1000, stop: 3000 })
    const bar = computeBar(tc, 1000, 10000)
    expect(bar.widthPct).toBe(20) // 2000 / 10000 * 100
  })

  it('computes correct left offset when bar does not start at minStart', () => {
    const tc = makeTestCase({ start: 3000, stop: 5000 })
    const bar = computeBar(tc, 1000, 10000)
    expect(bar.leftPct).toBe(20) // (3000-1000)/10000*100
    expect(bar.widthPct).toBe(20)
  })

  it('enforces minimum widthPct of 0.4 for very short tests', () => {
    const tc = makeTestCase({ start: 1000, stop: 1001 })
    const bar = computeBar(tc, 1000, 100000)
    expect(bar.widthPct).toBeGreaterThanOrEqual(0.4)
  })

  it('includes original test case reference on the bar', () => {
    const tc = makeTestCase({ name: 'ref-test' })
    const bar = computeBar(tc, 1000, 10000)
    expect(bar.tc).toBe(tc)
  })

  it('clamps leftPct to 0 when bar starts before window', () => {
    const tc = makeTestCase({ start: 500, stop: 2000 })
    const bar = computeBar(tc, 1000, 10000)
    expect(bar.leftPct).toBe(0)
  })
})

// ---------------------------------------------------------------------------
// stackBarsIntoRows
// ---------------------------------------------------------------------------
describe('stackBarsIntoRows', () => {
  it('returns empty array for no bars', () => {
    expect(stackBarsIntoRows([])).toEqual([])
  })

  it('places a single bar in one row', () => {
    const bar = computeBar(makeTestCase(), 1000, 10000)
    const rows = stackBarsIntoRows([bar])
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveLength(1)
  })

  it('places non-overlapping bars in the same row', () => {
    // bar1: 0–20%, bar2: 40–60%  — no overlap
    const tc1 = makeTestCase({ name: 'a', start: 1000, stop: 3000 })
    const tc2 = makeTestCase({ name: 'b', start: 5000, stop: 7000 })
    const bar1 = computeBar(tc1, 1000, 10000)
    const bar2 = computeBar(tc2, 1000, 10000)
    const rows = stackBarsIntoRows([bar1, bar2])
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveLength(2)
  })

  it('stacks overlapping bars into separate rows', () => {
    // bar1: 0–50%, bar2: 10–30%  — bar2 overlaps bar1
    const tc1 = makeTestCase({ name: 'a', start: 1000, stop: 6000 })
    const tc2 = makeTestCase({ name: 'b', start: 2000, stop: 4000 })
    const bar1 = computeBar(tc1, 1000, 10000)
    const bar2 = computeBar(tc2, 1000, 10000)
    const rows = stackBarsIntoRows([bar1, bar2])
    expect(rows).toHaveLength(2)
  })

  it('reuses the first available row (greedy)', () => {
    // bar1: 0–20%  -> row 0
    // bar2: 5–15%  -> row 1 (overlaps bar1)
    // bar3: 40–60% -> row 0 (fits after bar1, no overlap with bar2 in row 1)
    const tc1 = makeTestCase({ name: 'a', start: 1000, stop: 3000 })
    const tc2 = makeTestCase({ name: 'b', start: 1500, stop: 2500 })
    const tc3 = makeTestCase({ name: 'c', start: 5000, stop: 7000 })
    const bar1 = computeBar(tc1, 1000, 10000)
    const bar2 = computeBar(tc2, 1000, 10000)
    const bar3 = computeBar(tc3, 1000, 10000)
    const rows = stackBarsIntoRows([bar1, bar2, bar3])
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveLength(2) // bar1 + bar3
    expect(rows[1]).toHaveLength(1) // bar2
  })

  it('preserves bar order within each row', () => {
    const tc1 = makeTestCase({ name: 'a', start: 1000, stop: 2000 })
    const tc2 = makeTestCase({ name: 'b', start: 3000, stop: 4000 })
    const bar1 = computeBar(tc1, 1000, 10000)
    const bar2 = computeBar(tc2, 1000, 10000)
    const rows = stackBarsIntoRows([bar1, bar2])
    expect(rows[0][0].tc.name).toBe('a')
    expect(rows[0][1].tc.name).toBe('b')
  })
})
