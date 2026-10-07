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

describe('mergeRunFailures cluster key', () => {
  it('carries cluster_key onto the merged failure as clusterKey', () => {
    const [merged] = mergeRunFailures([failure({ error_message: 'boom', cluster_key: 'k-boom' })])
    expect(merged?.clusterKey).toBe('k-boom')
  })

  it('leaves clusterKey undefined when the API sent none', () => {
    const [merged] = mergeRunFailures([failure({ error_message: 'boom' })])
    expect(merged?.clusterKey).toBeUndefined()
  })

  // The key describes the message, so it travels with the copy that supplies it.
  it('takes the key of the twin that supplies the message', () => {
    const [merged] = mergeRunFailures([
      failure({ history_id: 'a', error_message: '', cluster_key: 'k-empty' }),
      failure({ history_id: 'b', error_message: 'boom', cluster_key: 'k-boom' }),
    ])
    expect(merged?.errorMessage).toBe('boom')
    expect(merged?.clusterKey).toBe('k-boom')
  })

  it('keeps its own key when the first twin already has a message', () => {
    const [merged] = mergeRunFailures([
      failure({ history_id: 'a', error_message: 'boom', cluster_key: 'k-boom' }),
      failure({ history_id: 'b', error_message: 'other', cluster_key: 'k-other' }),
    ])
    expect(merged?.errorMessage).toBe('boom')
    expect(merged?.clusterKey).toBe('k-boom')
  })
})
