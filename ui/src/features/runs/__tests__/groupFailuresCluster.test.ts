import { describe, it, expect } from 'vitest'

import { groupByError } from '../groupFailures'
import type { MergedFailure } from '../mergeRunFailures'

function makeFailure(overrides?: Partial<MergedFailure>): MergedFailure {
  return {
    key: 'k',
    projectId: 1,
    slug: 'ui-tests',
    buildId: 10,
    buildNumber: 1,
    testName: 't',
    fullName: 't',
    historyId: 'h',
    flaky: false,
    retries: 0,
    newFailed: false,
    known: false,
    errorMessage: 'boom',
    ...overrides,
  }
}

describe('groupByError by the server cluster key', () => {
  it('keeps messages that differ only by a number apart when their keys differ', () => {
    const groups = groupByError([
      makeFailure({ key: 'a', errorMessage: 'Timed out 5000ms', clusterKey: 'k-5000' }),
      makeFailure({ key: 'b', errorMessage: 'Timed out 10000ms', clusterKey: 'k-10000' }),
    ])
    expect(groups).toHaveLength(2)
    expect(groups.map((g) => g.title).sort()).toEqual(['Timed out 10000ms', 'Timed out 5000ms'])
  })

  it('joins different messages that share a key, titled by the first row first line', () => {
    const [group, ...rest] = groupByError([
      makeFailure({
        key: 'a',
        errorMessage: 'Error: connect ECONNREFUSED 10.0.0.1\n    at net.js:1:1',
        clusterKey: 'k-conn',
      }),
      makeFailure({
        key: 'b',
        projectId: 2,
        slug: 'api-tests',
        errorMessage: 'Error: socket hang up',
        clusterKey: 'k-conn',
      }),
    ])
    expect(rest).toHaveLength(0)
    expect(group?.rows).toHaveLength(2)
    expect(group?.title).toBe('Error: connect ECONNREFUSED 10.0.0.1')
    expect(group?.message).toBe('Error: connect ECONNREFUSED 10.0.0.1')
    expect(group?.subtitle).toBe('×2 · ui-tests, api-tests')
  })

  it('titles the group by the first row that has a message', () => {
    const [group] = groupByError([
      makeFailure({ key: 'a', errorMessage: '', clusterKey: 'k-x' }),
      makeFailure({ key: 'b', errorMessage: '\n  Real failure\nstack', clusterKey: 'k-x' }),
    ])
    expect(group?.title).toBe('Real failure')
    expect(group?.message).toBe('Real failure')
  })

  it('keys the group as error-<cluster key>', () => {
    const [group] = groupByError([makeFailure({ clusterKey: 'a1b2c3d4e5f60718' })])
    expect(group?.key).toBe('error-a1b2c3d4e5f60718')
  })

  it('falls back to exact-message grouping for rows without a key', () => {
    const groups = groupByError([
      makeFailure({ key: 'a', errorMessage: 'Timed out 5000ms' }),
      makeFailure({ key: 'b', errorMessage: 'Timed out 5000ms' }),
      makeFailure({ key: 'c', errorMessage: 'Timed out 10000ms' }),
    ])
    expect(groups).toHaveLength(2)
    expect(groups[0]?.rows).toHaveLength(2)
    expect(groups[0]?.key).toBe('error-Timed out 5000ms')
    expect(groups[1]?.key).toBe('error-Timed out 10000ms')
  })

  it('titles a group with no message "No error message" and sets no message', () => {
    const [group] = groupByError([
      makeFailure({ key: 'a', errorMessage: '', clusterKey: 'k-none' }),
      makeFailure({ key: 'b', errorMessage: '  \n ', clusterKey: 'k-none' }),
    ])
    expect(group?.title).toBe('No error message')
    expect(group?.message).toBeUndefined()
    expect(group?.rows).toHaveLength(2)
  })
})
