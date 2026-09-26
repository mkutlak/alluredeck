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
  it('collapses copies sharing a full_name into the copy carrying the error message', () => {
    const merged = mergeRunFailures([
      failure({
        history_id: '1ab6c50a.d93c9637',
        build_id: 1,
        build_number: 1,
        retries: 3,
        flaky: true,
      }),
      failure({
        history_id: '462170f6:d93c9637',
        build_id: 2,
        build_number: 2,
        error_message: 'Timed out 5000ms',
        new_failed: true,
        known: true,
      }),
    ])

    expect(merged).toHaveLength(1)
    expect(merged[0]).toMatchObject({
      testName: 'Set all as read in notification icon',
      // The copy with the message is the useful place to send the user.
      errorMessage: 'Timed out 5000ms',
      buildId: 2,
      buildNumber: 2,
      historyId: '462170f6:d93c9637',
      // The highest retry count, so the retry badge is not understated.
      retries: 3,
      // Stability and known flags are ORed across copies.
      flaky: true,
      newFailed: true,
      known: true,
    })
  })

  it.each([
    {
      // Two suites can legitimately hold a test at the same spec path.
      name: 'same full_name, different suites',
      rows: [
        failure({ project_id: 89, history_id: 'a' }),
        failure({ project_id: 200, slug: 'ui-user-groups', history_id: 'b' }),
      ],
      want: 2,
    },
    {
      name: 'empty full_name',
      rows: [
        failure({ full_name: '', test_name: 'same test', history_id: 'a' }),
        failure({ full_name: '', test_name: 'same test', history_id: 'b' }),
        failure({ full_name: '', test_name: 'other test', history_id: 'c' }),
      ],
      want: 2,
    },
  ])('keys rows by suite and full_name, falling back to test_name ($name)', ({ rows, want }) => {
    expect(mergeRunFailures(rows)).toHaveLength(want)
  })

  it('preserves input order with a unique key per merged row', () => {
    const merged = mergeRunFailures([
      failure({ full_name: 'b.spec.js:1:1', history_id: 'b' }),
      failure({ full_name: 'a.spec.js:1:1', history_id: 'a' }),
    ])

    expect(merged.map((m) => m.fullName)).toEqual(['b.spec.js:1:1', 'a.spec.js:1:1'])
    expect(new Set(merged.map((m) => m.key)).size).toBe(2)
  })
})
