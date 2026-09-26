import { describe, it, expect, afterEach } from 'vitest'
import {
  formatDate,
  formatDuration,
  calcPassRate,
  formatPassRate,
  getStatusVariant,
  truncate,
  formatBytes,
} from './utils'
import { useUIStore } from '@/store/ui'

describe('formatDuration', () => {
  it.each([
    [45_000, '45s'],
    [125_000, '2m 5s'],
    [120_000, '2m'],
    [3_660_000, '1h 1m'],
  ])('formatDuration(%d) is %s', (ms, expected) => {
    expect(formatDuration(ms)).toBe(expected)
  })
})

type Counts = [passed: number, total: number, skipped?: number]

// Pass rate excludes skipped tests from the denominator; broken still counts
// against it; with nothing left to rate (all skipped) there is no rate.
describe('calcPassRate', () => {
  it.each<[string, Counts, number | null]>([
    ['nothing ran', [0, 0], null],
    ['a plain ratio', [90, 100], 90],
    ['all passed', [50, 50], 100],
    ['skipped excluded: 31 passed / 36 total / 5 skipped', [31, 36, 5], 100],
    ['all skipped', [0, 5, 5], null],
  ])('%s -> %s', (_, args, expected) => {
    expect(calcPassRate(...args)).toBe(expected)
  })

  // Full precision, no rounding (fix bb96c92).
  it.each<[Counts, number]>([
    [[1, 3], 33.333],
    [[737, 740], 99.594],
    [[30, 36, 5], (30 / 31) * 100],
  ])('calcPassRate(%j) keeps full precision (~%d)', (args, expected) => {
    expect(calcPassRate(...args)).toBeCloseTo(expected, 2)
  })
})

// Floors to two decimals so a run with failures never shows 100% (fix bb96c92).
describe('formatPassRate', () => {
  it.each<[string, Counts, string]>([
    ['the canonical bug case', [737, 740], '99.59%'],
    ['floors near-100% values rather than rounding up', [731, 735], '99.45%'],
    ['all passed', [50, 50], '100%'],
    ['all passed (small run)', [3, 3], '100%'],
    ['none passed', [0, 100], '0%'],
    ['total is 0', [0, 0], '—'],
    ['a tiny non-zero rate', [1, 740], '0.13%'],
    ['skipped excluded: 31 passed / 36 total / 5 skipped', [31, 36, 5], '100%'],
    ['broken counts against: 30 passed / 36 total / 5 skipped', [30, 36, 5], '96.77%'],
    ['all skipped', [0, 5, 5], '—'],
  ])('formats %s: %j -> %s', (_, args, expected) => {
    expect(formatPassRate(...args)).toBe(expected)
  })

  it.each([
    [99.594, '99.59%'],
    [100, '100%'],
    [0, '0%'],
    [99.999, '99.99%'], // floor guarantee
  ])('formats a precomputed rate %d as %s', (rate, expected) => {
    expect(formatPassRate(rate)).toBe(expected)
  })
})

describe('getStatusVariant', () => {
  it.each([
    ['passed', 'passed'],
    ['FAILED', 'failed'],
    ['broken', 'broken'],
    ['skipped', 'skipped'],
    ['unknown', 'default'],
  ])('maps %s to %s', (status, expected) => {
    expect(getStatusVariant(status)).toBe(expected)
  })
})

describe('truncate', () => {
  it.each([
    ['hello', 10, 'hello'],
    ['hello world long string', 10, 'hello wor…'],
    ['a'.repeat(50), undefined, `${'a'.repeat(39)}…`],
  ])('truncate(%j, %s) is %j', (str, maxLen, expected) => {
    expect(truncate(str, maxLen)).toBe(expected)
  })
})

describe('formatDate', () => {
  afterEach(() => {
    useUIStore.setState({ timezone: null, timeFormat: null })
  })

  const iso = '2026-06-15T12:00:00Z'
  it.each([
    ['a Date', new Date(iso)],
    ['an ISO string', iso],
    ['an epoch-ms number', Date.parse(iso)],
  ])('formats %s with year, month, day, time and a 12h marker by default', (_, input) => {
    expect(formatDate(input)).toMatch(/Jun \d{2}, 2026.*\d{2}:\d{2}\s(AM|PM)/)
  })

  it('timezone Asia/Tokyo shifts hour for known UTC timestamp', () => {
    useUIStore.setState({ timezone: 'Asia/Tokyo' })
    // 2026-01-01T00:00:00Z is 09:00 in Tokyo (UTC+9, no DST)
    expect(formatDate(new Date('2026-01-01T00:00:00Z'))).toMatch(/09/)
  })

  it.each([
    ['24h', false],
    ['12h', true],
  ] as const)('timeFormat %s controls the AM/PM marker', (timeFormat, hasMarker) => {
    useUIStore.setState({ timeFormat })
    expect(/AM|PM/.test(formatDate(new Date('2026-01-01T14:00:00Z')))).toBe(hasMarker)
  })
})

describe('formatBytes', () => {
  it('returns "0 B" for negative input', () => {
    expect(formatBytes(-1)).toBe('0 B')
  })

  it('clamps to last unit instead of returning undefined for very large input', () => {
    const result = formatBytes(1099511627776)
    expect(result).not.toContain('undefined')
    expect(result).toMatch(/\d/)
  })
})
