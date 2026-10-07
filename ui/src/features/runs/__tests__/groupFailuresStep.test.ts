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

describe('groupByError failed step', () => {
  it('gives two clusters with one message their own step, leaving the subtitle alone', () => {
    const groups = groupByError([
      makeFailure({
        key: 'a',
        errorMessage: 'locator.click: Timeout',
        clusterKey: 'k-save',
        failedStep: 'click Save',
      }),
      makeFailure({
        key: 'b',
        errorMessage: 'locator.click: Timeout',
        clusterKey: 'k-submit',
        failedStep: 'click Submit',
      }),
    ])
    expect(groups.map((g) => g.title)).toEqual(['locator.click: Timeout', 'locator.click: Timeout'])
    expect(groups.map((g) => g.step).sort()).toEqual(['click Save', 'click Submit'])
    expect(groups.map((g) => g.subtitle)).toEqual(['×1 · ui-tests', '×1 · ui-tests'])
  })

  it('keeps the suites-only subtitle, including the +N overflow, when a step is set', () => {
    const [group] = groupByError([
      makeFailure({ key: 'a', clusterKey: 'k-x', failedStep: 'click Save' }),
      makeFailure({
        key: 'b',
        projectId: 2,
        slug: 'api-tests',
        displayName: 'API tests',
        clusterKey: 'k-x',
      }),
      makeFailure({ key: 'c', projectId: 3, slug: 'e2e', clusterKey: 'k-x' }),
    ])
    expect(group?.step).toBe('click Save')
    expect(group?.subtitle).toBe('×3 · ui-tests, API tests +1')
  })

  it('uses the first non-empty step of the group', () => {
    const [group] = groupByError([
      makeFailure({ key: 'a', clusterKey: 'k-x', failedStep: '' }),
      makeFailure({ key: 'b', clusterKey: 'k-x', failedStep: 'click Save' }),
    ])
    expect(group?.step).toBe('click Save')
  })

  it('sets no step when no row has one', () => {
    const [group] = groupByError([
      makeFailure({ key: 'a', clusterKey: 'k-x' }),
      makeFailure({ key: 'b', projectId: 2, slug: 'api-tests', clusterKey: 'k-x', failedStep: '' }),
    ])
    expect(group?.step).toBeUndefined()
    expect(group?.subtitle).toBe('×2 · ui-tests, api-tests')
  })
})
