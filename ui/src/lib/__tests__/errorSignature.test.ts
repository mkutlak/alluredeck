import { describe, it, expect } from 'vitest'

import { NO_ERROR_SIGNATURE, errorSignature } from '../errorSignature'

describe('errorSignature', () => {
  it('collapses the same failure mode with different timeout values', () => {
    // The runs feed regularly shows 18+ of these in a single run, differing
    // only in the timeout value. They are one failure mode.
    const a = errorSignature('Timed out 5000ms waiting for expect(locator).toBeVisible()')
    const b = errorSignature('Timed out 10000ms waiting for expect(locator).toBeVisible()')
    expect(a).toBe(b)
  })

  it('collapses Playwright locator timeouts that differ only in duration', () => {
    expect(errorSignature('TimeoutError: locator.click: Timeout 10000ms exceeded')).toBe(
      errorSignature('TimeoutError: locator.click: Timeout 30000ms exceeded'),
    )
  })

  it('keeps genuinely different failure modes apart', () => {
    const timeout = errorSignature('TimeoutError: locator.click: Timeout 10000ms exceeded')
    const query = errorSignature('Failed to execute query: The DELETE statement conflicted')
    expect(timeout).not.toBe(query)
  })

  it('does not merge different locator methods', () => {
    expect(errorSignature('TimeoutError: locator.click: Timeout 10000ms exceeded')).not.toBe(
      errorSignature('TimeoutError: locator.evaluate: Timeout 10000ms exceeded'),
    )
  })

  it('normalises quoted literals so per-case values do not fragment a cluster', () => {
    expect(errorSignature(`Expected 'alice' to equal 'bob'`)).toBe(
      errorSignature(`Expected 'carol' to equal 'dave'`),
    )
    expect(errorSignature('Expected "alice" to equal "bob"')).toBe(
      errorSignature('Expected "carol" to equal "dave"'),
    )
  })

  it('uses only the first line', () => {
    expect(errorSignature('TimeoutError: boom\n  at foo.ts:12\n  at bar.ts:5')).toBe(
      errorSignature('TimeoutError: boom'),
    )
  })

  it('tolerates CRLF input', () => {
    expect(errorSignature('TimeoutError: boom\r\n  at foo.ts:12')).toBe(
      errorSignature('TimeoutError: boom'),
    )
  })

  it('collapses runs of whitespace', () => {
    expect(errorSignature('Expected    a   to  equal  b')).toBe(errorSignature('Expected a to equal b'))
  })

  it('buckets empty and whitespace-only messages together', () => {
    expect(errorSignature('')).toBe(NO_ERROR_SIGNATURE)
    expect(errorSignature('   ')).toBe(NO_ERROR_SIGNATURE)
  })

  it('caps very long messages so a signature stays readable', () => {
    const long = `AssertionError: ${'x'.repeat(500)}`
    expect(errorSignature(long).length).toBeLessThanOrEqual(120)
  })

  it('is stable for an already-normalised message', () => {
    const once = errorSignature('TimeoutError: locator.click: Timeout 10000ms exceeded')
    expect(errorSignature(once)).toBe(once)
  })
})
