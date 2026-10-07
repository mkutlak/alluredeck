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

describe('groupByError', () => {
  const rows = [
    makeFailure({ key: 'a', errorMessage: 'Timed out 5000ms waiting for x' }),
    makeFailure({
      key: 'b',
      projectId: 2,
      slug: 'api-tests',
      displayName: 'API tests',
      errorMessage: 'Timed out 10000ms waiting for x',
    }),
    makeFailure({
      key: 'c',
      projectId: 3,
      slug: 'e2e',
      errorMessage: 'Timed out 1ms waiting for x',
    }),
    makeFailure({ key: 'd', errorMessage: 'Connection refused' }),
  ]

  it('still groups on the normalised signature', () => {
    const [timeouts, refused] = groupByError(rows)
    expect(timeouts?.key).toBe('error-Timed out Nms waiting for x')
    expect(timeouts?.rows).toHaveLength(3)
    expect(refused?.rows).toHaveLength(1)
  })

  it('carries the first real message verbatim, not the signature', () => {
    const [timeouts, refused] = groupByError(rows)
    expect(timeouts?.message).toBe('Timed out 5000ms waiting for x')
    expect(refused?.message).toBe('Connection refused')
  })

  it('has no message for failures that carry none', () => {
    const [group] = groupByError([makeFailure({ errorMessage: '' })])
    expect(group?.message).toBeUndefined()
    expect(group?.title).toBe('No error message')
  })

  it('states the count and the suites it spans, naming at most two', () => {
    const [timeouts, refused] = groupByError(rows)
    expect(timeouts?.subtitle).toBe('×3 · ui-tests, API tests +1')
    expect(refused?.subtitle).toBe('×1 · ui-tests')
  })

  // A multi-line (Playwright) error made the whole stack the group title.
  it.each([
    {
      name: 'a stack trace',
      message: 'Error: boom\n    at a.js:1:1\n    at b.js:2:2',
      want: 'Error: boom',
    },
    {
      name: 'a leading blank line',
      message: '\n\n  Error: boom\r\n  at a.js:1',
      want: 'Error: boom',
    },
  ])('uses only the first non-empty line of $name, verbatim', ({ message, want }) => {
    const [group] = groupByError([makeFailure({ errorMessage: message })])
    expect(group?.message).toBe(want)
  })

  // Two projects can share a display name; they are two suites, not one.
  it('counts suites by project, even when two projects share a name', () => {
    const shared = (key: string, projectId: number, slug = 'tests') =>
      makeFailure({ key, projectId, slug, errorMessage: 'boom' })
    const [group] = groupByError([
      shared('a', 1),
      shared('b', 2),
      shared('c', 3, 'other'),
      shared('d', 1),
    ])
    expect(group?.subtitle).toBe('×4 · tests, tests +1')
  })
})
