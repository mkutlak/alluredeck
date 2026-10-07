import { describe, it, expect } from 'vitest'

import { suiteVerdict, type FailureGroup } from '../groupFailures'
import type { MergedFailure } from '../mergeRunFailures'
import type { PipelineSuite } from '@/types/api'

function row(key: string, overrides: Partial<MergedFailure> = {}): MergedFailure {
  return {
    key,
    projectId: 1,
    slug: 'api-cloud',
    buildId: 10,
    buildNumber: 1,
    testName: key,
    fullName: key,
    historyId: key,
    flaky: false,
    retries: 0,
    newFailed: false,
    known: false,
    errorMessage: 'boom',
    origin: 'inherited',
    ...overrides,
  }
}

function group(failed: number, rows: MergedFailure[]): FailureGroup {
  const suite: PipelineSuite = {
    project_id: 1,
    slug: 'api-cloud',
    build_number: 1,
    build_id: 10,
    pass_rate: 0,
    total: 10,
    failed,
    duration_ms: 1,
    status: 'degraded',
  }
  return { key: 'suite-1', title: 'api-cloud', rows, suite }
}

const flaky = (key: string, o: Partial<MergedFailure> = {}) => row(key, { flaky: true, ...o })

describe('suiteVerdict', () => {
  it.each([
    { name: 'all flaky or known, complete', g: group(2, [flaky('a'), row('b', { known: true })]) },
    // An environment failure is never new, so a flaky one stays tolerable.
    {
      name: 'a flaky env row new against its baseline',
      g: group(1, [flaky('a', { origin: 'new', environment: true })]),
    },
    { name: 'flaky with no origin (unknown)', g: group(1, [flaky('a', { origin: undefined })]) },
  ])('reads ⚠ for $name', ({ g }) => {
    expect(suiteVerdict(g, false)).toBe('tolerated')
  })

  it.each([
    // every -> some would pass this one.
    {
      name: 'one flaky row and one plain row',
      g: group(2, [flaky('a'), row('b')]),
      truncated: false,
    },
    // === -> >= would pass this one: two listed rows for one failed test.
    {
      name: 'more listed rows than failed tests',
      g: group(1, [flaky('a'), flaky('b')]),
      truncated: false,
    },
    {
      name: 'fewer listed rows than failed tests',
      g: group(3, [flaky('a'), flaky('b')]),
      truncated: false,
    },
    // "1 new" on the page must not sit beside a ⚠.
    {
      name: 'a flaky row that is new',
      g: group(1, [flaky('a', { origin: 'new' })]),
      truncated: false,
    },
    { name: 'a truncated response', g: group(1, [flaky('a')]), truncated: true },
    // Nothing listed is not "all flaky": an unreadable report is still a failure.
    { name: 'no rows at all', g: group(0, []), truncated: false },
    { name: 'no suite', g: { key: 'suite-1', title: 'x', rows: [flaky('a')] }, truncated: false },
  ])('reads ✗ for $name', ({ g, truncated }) => {
    expect(suiteVerdict(g, truncated)).toBe('failed')
  })
})
