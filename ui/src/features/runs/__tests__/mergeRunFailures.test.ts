import { describe, it, expect } from 'vitest'

import { mergeRunFailures } from '../mergeRunFailures'
import type { RunFailure } from '@/types/api'

function failure(overrides: Partial<RunFailure> = {}): RunFailure {
  return {
    project_id: 89,
    slug: 'ui-ready-to-print-notifications',
    build_id: 17479,
    build_number: 218,
    test_name: 'Set all as read in notification icon',
    full_name: 'tests/ui/features/Notificaitons/Notifications.feature.spec.js:93:7',
    status: 'failed',
    duration_ms: 1000,
    history_id: 'h1',
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: '',
    ...overrides,
  }
}

describe('mergeRunFailures', () => {
  // Two ingestion paths assign different history_ids to the same test — one
  // using a "." separator, one a ":" — so the same failure arrives twice, with
  // only one copy carrying the error message. full_name is what they share.
  it('collapses rows that share a full_name but not a history_id', () => {
    const merged = mergeRunFailures([
      failure({ history_id: '1ab6c50a.d93c9637', retries: 3, error_message: '' }),
      failure({ history_id: '462170f6:d93c9637', retries: 0, error_message: 'Timed out 5000ms' }),
    ])

    expect(merged).toHaveLength(1)
    expect(merged[0]?.testName).toBe('Set all as read in notification icon')
  })

  it('keeps the error message from whichever copy carries one', () => {
    const merged = mergeRunFailures([
      failure({ history_id: 'a', error_message: '' }),
      failure({ history_id: 'b', error_message: 'Timed out 5000ms' }),
    ])
    expect(merged[0]?.errorMessage).toBe('Timed out 5000ms')
  })

  it('adopts the build of the copy that carries the error message', () => {
    // The copy with the message is the useful place to send the user.
    const merged = mergeRunFailures([
      failure({ history_id: 'a', build_id: 1, build_number: 1, error_message: '' }),
      failure({ history_id: 'b', build_id: 2, build_number: 2, error_message: 'boom' }),
    ])
    expect(merged[0]?.buildId).toBe(2)
    expect(merged[0]?.buildNumber).toBe(2)
  })

  it('keeps the highest retry count so the retry badge is not understated', () => {
    const merged = mergeRunFailures([
      failure({ history_id: 'a', retries: 3 }),
      failure({ history_id: 'b', retries: 0 }),
    ])
    expect(merged[0]?.retries).toBe(3)
  })

  it('ORs the stability and known flags across copies', () => {
    const merged = mergeRunFailures([
      failure({ history_id: 'a', flaky: true, new_failed: false, known: false }),
      failure({ history_id: 'b', flaky: false, new_failed: true, known: true }),
    ])
    expect(merged[0]?.flaky).toBe(true)
    expect(merged[0]?.newFailed).toBe(true)
    expect(merged[0]?.known).toBe(true)
  })

  it('does not merge the same full_name across different suites', () => {
    // Two suites can legitimately hold a test at the same spec path.
    const merged = mergeRunFailures([
      failure({ project_id: 89, history_id: 'a' }),
      failure({ project_id: 200, slug: 'ui-user-groups', history_id: 'b' }),
    ])
    expect(merged).toHaveLength(2)
  })

  it('falls back to test_name when full_name is empty', () => {
    const merged = mergeRunFailures([
      failure({ full_name: '', test_name: 'same test', history_id: 'a' }),
      failure({ full_name: '', test_name: 'same test', history_id: 'b' }),
      failure({ full_name: '', test_name: 'other test', history_id: 'c' }),
    ])
    expect(merged).toHaveLength(2)
  })

  it('preserves input order', () => {
    const merged = mergeRunFailures([
      failure({ full_name: 'b.spec.js:1:1', history_id: 'b' }),
      failure({ full_name: 'a.spec.js:1:1', history_id: 'a' }),
    ])
    expect(merged.map((m) => m.fullName)).toEqual(['b.spec.js:1:1', 'a.spec.js:1:1'])
  })

  it('produces a stable unique key per merged row', () => {
    const merged = mergeRunFailures([
      failure({ project_id: 89, full_name: 'a.spec.js:1:1', history_id: 'a' }),
      failure({ project_id: 89, full_name: 'b.spec.js:1:1', history_id: 'b' }),
    ])
    const keys = merged.map((m) => m.key)
    expect(new Set(keys).size).toBe(keys.length)
  })

  it('returns an empty array for no input', () => {
    expect(mergeRunFailures([])).toEqual([])
  })
})
