import { describe, it, expect, afterEach } from 'vitest'
import {
  cn,
  formatDate,
  formatDuration,
  calcPassRate,
  formatPassRate,
  getStatusVariant,
  truncate,
  formatBytes,
} from './utils'
import { useUIStore } from '@/store/ui'

describe('cn', () => {
  it('merges class names', () => {
    expect(cn('foo', 'bar')).toBe('foo bar')
  })

  it('handles conditional classes', () => {
    const skip = false
    expect(cn('base', skip && 'skipped', 'included')).toBe('base included')
  })

  it('handles tailwind conflicts by keeping last', () => {
    expect(cn('p-4', 'p-2')).toBe('p-2')
  })
})

describe('formatDuration', () => {
  it('formats seconds', () => {
    expect(formatDuration(45_000)).toBe('45s')
  })

  it('formats minutes and seconds', () => {
    expect(formatDuration(125_000)).toBe('2m 5s')
  })

  it('formats hours and minutes', () => {
    expect(formatDuration(3_660_000)).toBe('1h 1m')
  })

  it('formats whole minutes', () => {
    expect(formatDuration(120_000)).toBe('2m')
  })
})

describe('calcPassRate', () => {
  it('returns null for total = 0', () => {
    expect(calcPassRate(0, 0)).toBeNull()
  })

  it('calculates percentage correctly', () => {
    expect(calcPassRate(90, 100)).toBe(90)
  })

  it('returns full precision (no rounding)', () => {
    expect(calcPassRate(1, 3)).toBeCloseTo(33.333, 2)
  })

  it('returns 100 when all passed', () => {
    expect(calcPassRate(50, 50)).toBe(100)
  })

  it('returns full precision for near-100% values', () => {
    expect(calcPassRate(737, 740)).toBeCloseTo(99.594, 2)
  })

  it('excludes skipped from denominator: 31 passed / 36 total / 5 skipped = 100', () => {
    expect(calcPassRate(31, 36, 5)).toBe(100)
  })

  it('broken counts against rate: 30 passed / 36 total / 5 skipped = 30/31', () => {
    expect(calcPassRate(30, 36, 5)).toBeCloseTo((30 / 31) * 100, 2)
  })

  it('all-skipped: effectiveTotal <= 0 returns null', () => {
    expect(calcPassRate(0, 5, 5)).toBeNull()
  })
})

describe('formatPassRate', () => {
  it('formats the canonical bug case (737/740) with 2 decimals', () => {
    expect(formatPassRate(737, 740)).toBe('99.59%')
  })

  it('floors near-100% values rather than rounding up', () => {
    // 731/735 = 99.4557… — floor to 99.45, not round to 99.46
    expect(formatPassRate(731, 735)).toBe('99.45%')
  })

  it('returns "100%" when all passed (exact)', () => {
    expect(formatPassRate(50, 50)).toBe('100%')
    expect(formatPassRate(3, 3)).toBe('100%')
  })

  it('returns "0%" when none passed', () => {
    expect(formatPassRate(0, 100)).toBe('0%')
  })

  it('returns "—" when total is 0', () => {
    expect(formatPassRate(0, 0)).toBe('—')
  })

  it('preserves tiny non-zero rates as 2-decimal output', () => {
    // 1/740 = 0.1351% — floor to 0.13
    expect(formatPassRate(1, 740)).toBe('0.13%')
  })

  it('accepts a precomputed rate via single-arg overload', () => {
    expect(formatPassRate(99.594)).toBe('99.59%')
  })

  it('returns "100%" for exact 100 via single-arg overload', () => {
    expect(formatPassRate(100)).toBe('100%')
  })

  it('returns "0%" for exact 0 via single-arg overload', () => {
    expect(formatPassRate(0)).toBe('0%')
  })

  it('never renders 100% for 99.999% (floor guarantee)', () => {
    expect(formatPassRate(99.999)).toBe('99.99%')
  })

  it('screenshot case: 31 passed / 36 total / 5 skipped = "100%"', () => {
    expect(formatPassRate(31, 36, 5)).toBe('100%')
  })

  it('broken counts against: 30 passed / 36 total / 5 skipped formats correctly', () => {
    expect(formatPassRate(30, 36, 5)).toBe('96.77%')
  })

  it('all-skipped: returns "—" when effectiveTotal <= 0', () => {
    expect(formatPassRate(0, 5, 5)).toBe('—')
  })
})

describe('getStatusVariant', () => {
  it('returns passed for "passed"', () => {
    expect(getStatusVariant('passed')).toBe('passed')
  })

  it('returns failed for "failed"', () => {
    expect(getStatusVariant('FAILED')).toBe('failed')
  })

  it('returns broken for "broken"', () => {
    expect(getStatusVariant('broken')).toBe('broken')
  })

  it('returns skipped for "skipped"', () => {
    expect(getStatusVariant('skipped')).toBe('skipped')
  })

  it('returns default for unknown status', () => {
    expect(getStatusVariant('unknown')).toBe('default')
  })
})

describe('truncate', () => {
  it('returns short strings unchanged', () => {
    expect(truncate('hello', 10)).toBe('hello')
  })

  it('truncates long strings with ellipsis', () => {
    const result = truncate('hello world long string', 10)
    expect(result).toHaveLength(10)
    expect(result.endsWith('…')).toBe(true)
  })

  it('uses default max length of 40', () => {
    const long = 'a'.repeat(50)
    const result = truncate(long)
    expect(result).toHaveLength(40)
  })
})

describe('formatDate', () => {
  afterEach(() => {
    useUIStore.setState({ timezone: null, timeFormat: null })
  })

  it('formats a timestamp', () => {
    const result = formatDate(new Date('2025-01-15T10:30:00Z'))
    expect(result).toMatch(/Jan/)
    expect(result).toMatch(/2025/)
  })

  it('formats a date string', () => {
    const result = formatDate('2025-06-01T00:00:00Z')
    expect(result).toMatch(/Jun/)
  })

  it('default state produces output with year, month, day, hour, minute and AM/PM marker', () => {
    const result = formatDate(new Date('2026-03-15T14:30:00Z'))
    expect(result).toMatch(/2026/)
    expect(result).toMatch(/Mar/)
    expect(result).toMatch(/\d{2}:\d{2}/)
    expect(result).toMatch(/AM|PM/)
  })

  it('timezone Asia/Tokyo shifts hour for known UTC timestamp', () => {
    useUIStore.setState({ timezone: 'Asia/Tokyo' })
    // 2026-01-01T00:00:00Z is 09:00 in Tokyo (UTC+9, no DST)
    const result = formatDate(new Date('2026-01-01T00:00:00Z'))
    expect(result).toMatch(/09/)
  })

  it('timeFormat 24h produces output without AM or PM', () => {
    useUIStore.setState({ timeFormat: '24h' })
    const result = formatDate(new Date('2026-01-01T14:00:00Z'))
    expect(result).not.toMatch(/AM|PM/)
  })

  it('timeFormat 12h produces output with AM or PM', () => {
    useUIStore.setState({ timeFormat: '12h' })
    const result = formatDate(new Date('2026-01-01T14:00:00Z'))
    expect(result).toMatch(/AM|PM/)
  })

  it('accepts a numeric timestamp', () => {
    const ts = new Date('2026-06-01T00:00:00Z').getTime()
    const result = formatDate(ts)
    expect(result).toMatch(/Jun/)
    expect(result).toMatch(/2026/)
  })
})

describe('formatBytes', () => {
  it('returns "0 B" for negative input', () => {
    expect(formatBytes(-1)).toBe('0 B')
  })

  it('returns "0 B" for large negative input', () => {
    expect(formatBytes(-100)).toBe('0 B')
  })

  it('clamps to last unit instead of returning undefined for very large input', () => {
    const result = formatBytes(1099511627776)
    expect(result).not.toContain('undefined')
    expect(result).toMatch(/\d/)
  })
})
