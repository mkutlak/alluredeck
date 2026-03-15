import { describe, it, expect } from 'vitest'
import { computeTicks, formatRelativeTime } from '../timelineHelpers'

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

