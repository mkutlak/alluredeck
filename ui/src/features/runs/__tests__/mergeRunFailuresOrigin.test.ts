import { describe, it, expect } from 'vitest'

import { mergeRunFailures } from '../mergeRunFailures'
import type { RunFailure } from '@/types/api'

function failure(overrides: Partial<RunFailure> = {}): RunFailure {
  return {
    project_id: 1,
    slug: 'ui-tests',
    build_id: 10,
    build_number: 1,
    test_name: 'logs in',
    full_name: 'login.spec.js:1:1',
    status: 'failed',
    duration_ms: 100,
    history_id: 'h1',
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: '',
    ...overrides,
  }
}

describe('mergeRunFailures origin', () => {
  // The two ingestion copies carry different history_ids, so only one may match the
  // baseline. As in the API's SQL, a failure in the baseline under either id wins
  // (inherited > new), and any known origin beats unknown.
  it.each([
    { origins: ['unknown', 'inherited'], want: 'inherited' },
    { origins: [undefined, 'new'], want: 'new' },
    { origins: ['inherited', 'new'], want: 'inherited' },
    { origins: ['new', 'inherited'], want: 'inherited' },
    { origins: ['new', 'unknown'], want: 'new' },
  ] as const)('merges origins $origins into $want', ({ origins, want }) => {
    const [first, second] = origins
    const merged = mergeRunFailures([
      failure({ history_id: 'a', origin: first }),
      failure({ history_id: 'b', origin: second }),
    ])
    expect(merged).toHaveLength(1)
    expect(merged[0]?.origin).toBe(want)
  })

  it('lets a real origin replace an unexpected one', () => {
    const merged = mergeRunFailures([
      failure({ history_id: 'a', origin: '' as unknown as RunFailure['origin'] }),
      failure({ history_id: 'b', origin: 'inherited' }),
    ])
    expect(merged[0]?.origin).toBe('inherited')
  })

  it('ORs the environment flag across copies', () => {
    const merged = mergeRunFailures([
      failure({ history_id: 'a', environment: false }),
      failure({ history_id: 'b', environment: true }),
    ])
    expect(merged[0]?.environment).toBe(true)
  })

  // The runs feature stops reading new_failed: "new" is the effective bucket only.
  it.each([
    { row: { origin: 'new' as const }, want: true },
    { row: { origin: 'new' as const, environment: true }, want: false },
    { row: { origin: 'inherited' as const, new_failed: true }, want: false },
    { row: { new_failed: true }, want: false },
  ])('derives newFailed $want from $row, never from new_failed', ({ row, want }) => {
    expect(mergeRunFailures([failure(row)])[0]?.newFailed).toBe(want)
  })
})
