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

describe('mergeRunFailures failed step', () => {
  it('carries failed_step onto the merged failure as failedStep', () => {
    const [merged] = mergeRunFailures([
      failure({ error_message: 'boom', failed_step: 'click Save' }),
    ])
    expect(merged?.failedStep).toBe('click Save')
  })

  it('leaves failedStep undefined when the API sent none', () => {
    const [merged] = mergeRunFailures([failure({ error_message: 'boom' })])
    expect(merged?.failedStep).toBeUndefined()
  })

  // The step describes the failure the message came from, so it travels with that copy.
  it('takes the step of the twin that supplies the message', () => {
    const [merged] = mergeRunFailures([
      failure({ history_id: 'a', error_message: '', failed_step: 'click Submit' }),
      failure({ history_id: 'b', error_message: 'boom', failed_step: 'click Save' }),
    ])
    expect(merged?.errorMessage).toBe('boom')
    expect(merged?.failedStep).toBe('click Save')
  })

  it('keeps its own step when the first twin already has a message', () => {
    const [merged] = mergeRunFailures([
      failure({ history_id: 'a', error_message: 'boom', failed_step: 'click Save' }),
      failure({ history_id: 'b', error_message: 'other', failed_step: 'click Submit' }),
    ])
    expect(merged?.errorMessage).toBe('boom')
    expect(merged?.failedStep).toBe('click Save')
  })
})
