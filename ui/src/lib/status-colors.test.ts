import { describe, it, expect } from 'vitest'
import { STATUS_TEXT_CLASSES, getPassRateColorClass, getPassRateBadgeClass } from './status-colors'

// Thresholds: >= 90 green, >= 70 orange, below 70 red (badge falls back to the
// default destructive variant).
describe('pass-rate thresholds', () => {
  it.each([
    [100, 'passed', '#40a02b'],
    [90, 'passed', '#40a02b'],
    [89, 'broken', '#fe640b'],
    [70, 'broken', '#fe640b'],
    [69, 'failed', undefined],
    [0, 'failed', undefined],
  ] as const)('rate %d -> %s text, badge colour %s', (rate, status, badgeHex) => {
    expect(getPassRateColorClass(rate)).toBe(STATUS_TEXT_CLASSES[status])
    const badge = getPassRateBadgeClass(rate)
    if (badgeHex) expect(badge).toContain(badgeHex)
    else expect(badge).toBeUndefined()
  })
})
