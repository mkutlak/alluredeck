import { describe, it, expect } from 'vitest'
import {
  cn,
  formatDate,
  formatDuration,
  calcPassRate,
  getStatusVariant,
  truncate,
  formatBytes,
} from './utils'

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
  it('returns 0 for total = 0', () => {
    expect(calcPassRate(0, 0)).toBe(0)
  })

  it('calculates percentage correctly', () => {
    expect(calcPassRate(90, 100)).toBe(90)
  })

  it('rounds to nearest integer', () => {
    expect(calcPassRate(1, 3)).toBe(33)
  })

  it('returns 100 when all passed', () => {
    expect(calcPassRate(50, 50)).toBe(100)
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
  it('formats a timestamp', () => {
    const result = formatDate(new Date('2025-01-15T10:30:00Z'))
    expect(result).toMatch(/Jan/)
    expect(result).toMatch(/2025/)
  })

  it('formats a date string', () => {
    const result = formatDate('2025-06-01T00:00:00Z')
    expect(result).toMatch(/Jun/)
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
