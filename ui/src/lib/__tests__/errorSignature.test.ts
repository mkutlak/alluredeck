import { describe, it, expect } from 'vitest'

import { NO_ERROR_SIGNATURE, errorSignature } from '../errorSignature'

describe('errorSignature', () => {
  // The runs feed regularly shows 18+ messages in one run that differ only in
  // a timeout value or a quoted per-case literal. They are one failure mode.
  it.each([
    [
      'durations',
      'TimeoutError: locator.click: Timeout 10000ms exceeded',
      'TimeoutError: locator.click: Timeout 30000ms exceeded',
    ],
    [
      'single-quoted literals',
      `Expected 'alice' to equal 'bob'`,
      `Expected 'carol' to equal 'dave'`,
    ],
    [
      'double-quoted literals',
      'Expected "alice" to equal "bob"',
      'Expected "carol" to equal "dave"',
    ],
    [
      'lines after the first',
      'TimeoutError: boom\n  at foo.ts:12\n  at bar.ts:5',
      'TimeoutError: boom',
    ],
    ['whitespace runs', 'Expected    a   to  equal  b', 'Expected a to equal b'],
  ])('collapses messages that differ only in %s', (_, a, b) => {
    expect(errorSignature(a)).toBe(errorSignature(b))
  })

  it('does not merge different locator methods', () => {
    expect(errorSignature('TimeoutError: locator.click: Timeout 10000ms exceeded')).not.toBe(
      errorSignature('TimeoutError: locator.evaluate: Timeout 10000ms exceeded'),
    )
  })

  it('buckets empty and whitespace-only messages together', () => {
    expect(errorSignature('')).toBe(NO_ERROR_SIGNATURE)
    expect(errorSignature('   ')).toBe(NO_ERROR_SIGNATURE)
  })

  it('caps very long messages so a signature stays readable', () => {
    const long = `AssertionError: ${'x'.repeat(500)}`
    expect(errorSignature(long).length).toBeLessThanOrEqual(120)
  })
})
