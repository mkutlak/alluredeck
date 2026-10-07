import { act, screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { formatDate } from '@/lib/utils'
import { formatRelativeTime, splitBranch } from '../rowFormat'
import { RunRow } from '../RunRow'
import type { PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn().mockResolvedValue({ data: [], metadata: { message: 'ok' } }),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({ data: [], metadata: { message: 'ok' } }),
  getProjects: vi.fn(),
}))

const HOUR = 3600e3
const LONG_BRANCH =
  'feature/PAY-99999-multi-region-settlement-reconciliation-with-idempotency-keys-and-retry-budget'

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: new Date(Date.now() - 3 * HOUR - 1000).toISOString(),
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      {
        project_id: 1,
        slug: 'api-cloud',
        build_number: 5,
        build_id: 105,
        pass_rate: 100,
        total: 42,
        failed: 0,
        duration_ms: 15000,
        status: 'passed',
        builds: [{ build_id: 105, build_number: 5 }],
      },
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
    },
    ...overrides,
  }
}

describe('splitBranch', () => {
  it.each(['main', 'release/2.4', 'feature/one-failure'])(
    'keeps the short branch %s whole, with no tail',
    (branch) => {
      expect(splitBranch(branch)).toEqual([branch, ''])
    },
  )

  it('splits a long branch into a head and a short tail that rejoin to the original', () => {
    const [head, tail] = splitBranch(LONG_BRANCH)
    expect(head + tail).toBe(LONG_BRANCH)
    expect(tail).not.toBe('')
    expect(tail.length).toBeLessThanOrEqual(16)
    expect(LONG_BRANCH.endsWith(tail)).toBe(true)
  })
})

describe('formatRelativeTime', () => {
  const now = Date.parse('2026-10-07T12:00:00Z')
  const ago = (ms: number) => new Date(now - ms).toISOString()

  it.each([
    { ms: 0, text: 'now' },
    { ms: 30e3, text: '30 seconds ago' },
    { ms: 5 * 60e3, text: '5 minutes ago' },
    { ms: 3 * HOUR, text: '3 hours ago' },
    // Floors, never rounds up into the next unit.
    { ms: 59.9 * 60e3, text: '59 minutes ago' },
    { ms: 2 * 24 * HOUR, text: '2 days ago' },
    { ms: 60 * 24 * HOUR, text: '2 months ago' },
    // Elapsed time, not calendar days: 47 hours ago is not "yesterday", 59 days is
    // not "last month", 729 days is not "last year".
    { ms: 47 * HOUR, text: '1 day ago' },
    { ms: 59 * 24 * HOUR, text: '1 month ago' },
    { ms: 729 * 24 * HOUR, text: '1 year ago' },
  ])('says "$text" for a run $ms ms old', ({ ms, text }) => {
    expect(formatRelativeTime(ago(ms), now)).toBe(text)
  })

  // A skewed CI clock can stamp a run slightly in the future.
  it('treats a timestamp in the future as "now"', () => {
    expect(formatRelativeTime(ago(-5000), now)).toBe('now')
  })
})

describe('RunRow line 1', () => {
  it('shows the run time relative, with the full date as its tooltip', () => {
    const run = makeRun()
    renderWithProviders(<RunRow run={run} />)
    const time = screen.getByText('3 hours ago')
    expect(time).toHaveAttribute('title', formatDate(run.timestamp))
    expect(time).toHaveAttribute('datetime', run.timestamp)
  })

  // The branch is split into a truncating head and a visible tail; the two must
  // read, and copy, as the one branch name, not twice and not in two lines.
  it('keeps one copy of a long branch, in a single element', () => {
    renderWithProviders(<RunRow run={makeRun({ branch: LONG_BRANCH })} />)
    expect(screen.getByTitle(LONG_BRANCH).textContent).toBe(LONG_BRANCH)
  })

  // The label is a function of the clock; React Query will not re-render an
  // unchanged feed, so a feed left open has to tick on its own.
  it('refreshes the relative time while the page stays open', () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
    try {
      const now = Date.parse('2026-10-07T12:00:00Z')
      vi.setSystemTime(now)
      renderWithProviders(
        <RunRow run={makeRun({ timestamp: new Date(now - 59.5 * 60e3).toISOString() })} />,
      )
      expect(screen.getByText('59 minutes ago')).toBeInTheDocument()
      act(() => {
        vi.advanceTimersByTime(60e3)
      })
      expect(screen.getByText('1 hour ago')).toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })
})
