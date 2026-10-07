import { describe, it, expect } from 'vitest'
import { AWAITING_POLL_MS, pollWhileAwaiting } from './pipeline'
import type { PipelineRun } from '@/types/api'

function run(awaiting_suites?: number): PipelineRun {
  return {
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    suites: [],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
      awaiting_suites,
    },
  }
}

const query = (
  data: { data: PipelineRun | PipelineRun[] } | undefined,
  status: 'pending' | 'error' | 'success' = 'success',
) => ({ state: { status, data } })

describe('pollWhileAwaiting', () => {
  it('polls while the single run it holds is still awaiting suites', () => {
    expect(pollWhileAwaiting(query({ data: run(3) }))).toBe(AWAITING_POLL_MS)
  })

  it('polls while any run in the list is awaiting suites', () => {
    expect(pollWhileAwaiting(query({ data: [run(), run(0), run(1)] }))).toBe(AWAITING_POLL_MS)
  })

  it.each([undefined, 0])('stops when awaiting_suites is %s', (awaiting) => {
    expect(pollWhileAwaiting(query({ data: run(awaiting) }))).toBe(false)
    expect(pollWhileAwaiting(query({ data: [run(awaiting), run(awaiting)] }))).toBe(false)
  })

  // An outage or a deleted run would otherwise log a failure every 30 s; a refetch that
  // succeeds (focus, remount) starts the polling again.
  it('stops while the query is in error, even though it still holds an awaiting run', () => {
    expect(pollWhileAwaiting(query({ data: run(3) }, 'error'))).toBe(false)
    expect(pollWhileAwaiting(query({ data: [run(3)] }, 'error'))).toBe(false)
  })

  it('does not poll an empty list or a query with no data yet', () => {
    expect(pollWhileAwaiting(query({ data: [] }))).toBe(false)
    expect(pollWhileAwaiting(query(undefined))).toBe(false)
  })
})
