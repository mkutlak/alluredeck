import { describe, it, expect } from 'vitest'
import {
  STATUS_COLORS,
  STATUS_DARK_COLORS,
  STATUS_TEXT_CLASSES,
  STATUS_BADGE_CLASSES,
  CATEGORY_COLORS,
  CATEGORY_DEFAULT_COLOR,
  getPassRateColorClass,
  getPassRateBadgeClass,
} from './status-colors'

describe('STATUS_COLORS', () => {
  it('has all 4 light-mode hex values', () => {
    expect(STATUS_COLORS.passed).toBe('#40a02b')
    expect(STATUS_COLORS.failed).toBe('#d20f39')
    expect(STATUS_COLORS.broken).toBe('#fe640b')
    expect(STATUS_COLORS.skipped).toBe('#8c8fa1')
  })
})

describe('STATUS_DARK_COLORS', () => {
  it('has all 4 dark-mode hex values', () => {
    expect(STATUS_DARK_COLORS.passed).toBe('#a6e3a1')
    expect(STATUS_DARK_COLORS.failed).toBe('#f38ba8')
    expect(STATUS_DARK_COLORS.broken).toBe('#fab387')
    expect(STATUS_DARK_COLORS.skipped).toBe('#a6adc8')
  })
})

describe('STATUS_TEXT_CLASSES', () => {
  it('passed uses green light + dark text', () => {
    expect(STATUS_TEXT_CLASSES.passed).toBe('text-[#40a02b] dark:text-[#a6e3a1]')
  })
  it('failed uses red light + dark text', () => {
    expect(STATUS_TEXT_CLASSES.failed).toBe('text-[#d20f39] dark:text-[#f38ba8]')
  })
  it('broken uses orange light + dark text', () => {
    expect(STATUS_TEXT_CLASSES.broken).toBe('text-[#fe640b] dark:text-[#fab387]')
  })
  it('skipped uses subtext0 light + dark text', () => {
    expect(STATUS_TEXT_CLASSES.skipped).toBe('text-[#6c6f85] dark:text-[#a6adc8]')
  })
})

describe('STATUS_BADGE_CLASSES', () => {
  it('passed has border-transparent bg and text classes', () => {
    expect(STATUS_BADGE_CLASSES.passed).toBe(
      'border-transparent bg-[#40a02b]/15 text-[#40a02b] dark:bg-[#a6e3a1]/15 dark:text-[#a6e3a1]',
    )
  })
  it('failed has correct badge classes', () => {
    expect(STATUS_BADGE_CLASSES.failed).toBe(
      'border-transparent bg-[#d20f39]/15 text-[#d20f39] dark:bg-[#f38ba8]/15 dark:text-[#f38ba8]',
    )
  })
  it('broken has correct badge classes', () => {
    expect(STATUS_BADGE_CLASSES.broken).toBe(
      'border-transparent bg-[#fe640b]/15 text-[#fe640b] dark:bg-[#fab387]/15 dark:text-[#fab387]',
    )
  })
  it('skipped has correct badge classes', () => {
    expect(STATUS_BADGE_CLASSES.skipped).toBe(
      'border-transparent bg-[#8c8fa1]/15 text-[#6c6f85] dark:bg-[#7f849c]/15 dark:text-[#a6adc8]',
    )
  })
})

describe('CATEGORY_COLORS', () => {
  it('has product defects color', () => {
    expect(CATEGORY_COLORS['Product defects']).toBe('#d20f39')
  })
  it('has test defects color', () => {
    expect(CATEGORY_COLORS['Test defects']).toBe('#fe640b')
  })
})

describe('CATEGORY_DEFAULT_COLOR', () => {
  it('is overlay1 gray', () => {
    expect(CATEGORY_DEFAULT_COLOR).toBe('#8c8fa1')
  })
})

describe('getPassRateColorClass', () => {
  it('returns passed class for rate >= 90', () => {
    expect(getPassRateColorClass(90)).toBe(STATUS_TEXT_CLASSES.passed)
    expect(getPassRateColorClass(100)).toBe(STATUS_TEXT_CLASSES.passed)
  })
  it('returns broken class for rate >= 70 and < 90', () => {
    expect(getPassRateColorClass(70)).toBe(STATUS_TEXT_CLASSES.broken)
    expect(getPassRateColorClass(89)).toBe(STATUS_TEXT_CLASSES.broken)
  })
  it('returns failed class for rate < 70', () => {
    expect(getPassRateColorClass(0)).toBe(STATUS_TEXT_CLASSES.failed)
    expect(getPassRateColorClass(69)).toBe(STATUS_TEXT_CLASSES.failed)
  })
})

describe('getPassRateBadgeClass', () => {
  it('returns green badge for rate >= 90', () => {
    const cls = getPassRateBadgeClass(95)
    expect(cls).toContain('#40a02b')
  })
  it('returns orange badge for rate >= 70 and < 90', () => {
    const cls = getPassRateBadgeClass(75)
    expect(cls).toContain('#fe640b')
  })
  it('returns undefined or empty for low rates (uses default destructive variant)', () => {
    const cls = getPassRateBadgeClass(50)
    expect(cls).toBeUndefined()
  })
})
