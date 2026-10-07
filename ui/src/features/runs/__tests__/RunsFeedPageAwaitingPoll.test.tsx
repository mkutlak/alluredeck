import { act, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunsFeedPage } from '../RunsFeedPage'
import type { PaginatedResponse, PipelineRun, RunFailure } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunsFeed: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunFailures: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', parent_id: null, children: [1, 2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({ fetchBranches: vi.fn().mockResolvedValue([]) }))

import { fetchRunFailures, fetchRunsFeed } from '@/api/pipeline'

// Suites as [project id, failed tests]. A newer upload moves the run's timestamp.
function makeRun(
  awaiting?: number,
  suites: Array<[number, number]> = [[1, 0]],
  timestamp = '2026-04-03T18:00:00Z',
): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp,
    group_project_id: 10,
    group_slug: 'acme',
    suites: suites.map(([id, failed]) => ({
      project_id: id,
      slug: id === 1 ? 'api-cloud' : 'ui-tests',
      build_number: 5,
      build_id: 100 + id,
      pass_rate: 100,
      total: 42,
      failed,
      duration_ms: 15000,
      status: failed > 0 ? 'degraded' : 'passed',
    })),
    aggregate: {
      suites_passed: suites.filter(([, failed]) => failed === 0).length,
      suites_total: suites.length,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
      awaiting_suites: awaiting,
    },
  }
}

function failure(project_id: number, test_name: string): RunFailure {
  return {
    project_id,
    slug: project_id === 1 ? 'api-cloud' : 'ui-tests',
    build_id: 100 + project_id,
    build_number: 5,
    test_name,
    full_name: test_name,
    status: 'failed',
    duration_ms: 100,
    history_id: `h-${test_name}`,
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'AssertionError: boom',
  }
}

const failuresOf = (...rows: RunFailure[]) => ({
  data: rows,
  metadata: { message: 'ok', truncated: false },
})

const feed = (run: PipelineRun): PaginatedResponse<PipelineRun[]> => ({
  data: [run],
  metadata: { message: 'ok' },
  pagination: { page: 1, per_page: 10, total: 1, total_pages: 1 },
})

// Only the interval clock is faked. Testing Library's waitFor polls with setInterval too,
// so findBy* resolves on its first check or when the DOM changes (MutationObserver), which
// is all these waits need: each one is for something the render is about to show.
const advance = (ms: number) =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })

describe('RunsFeedPage polling for a run that is still uploading', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    vi.mocked(fetchRunsFeed).mockReset()
    vi.mocked(fetchRunFailures).mockReset()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('refetches every 30 s while a run awaits suites, then stops once none does', async () => {
    vi.mocked(fetchRunsFeed)
      .mockResolvedValueOnce(feed(makeRun(2)))
      .mockResolvedValue(feed(makeRun()))
    renderWithProviders(<RunsFeedPage />)

    expect(await screen.findByText('awaiting 2 suites')).toBeInTheDocument()
    expect(fetchRunsFeed).toHaveBeenCalledTimes(1)

    await advance(29_000)
    expect(fetchRunsFeed).toHaveBeenCalledTimes(1)
    await advance(1_000)
    expect(fetchRunsFeed).toHaveBeenCalledTimes(2)
    expect(await screen.findByText('1/1 suites passed')).toBeInTheDocument()

    await advance(120_000)
    expect(fetchRunsFeed).toHaveBeenCalledTimes(2)
  })

  it('keeps polling while the refetched run is still awaiting suites', async () => {
    vi.mocked(fetchRunsFeed)
      .mockResolvedValueOnce(feed(makeRun(2)))
      .mockResolvedValueOnce(feed(makeRun(1)))
      .mockResolvedValue(feed(makeRun()))
    renderWithProviders(<RunsFeedPage />)

    expect(await screen.findByText('awaiting 2 suites')).toBeInTheDocument()
    await advance(30_000)
    expect(await screen.findByText('awaiting 1 suite')).toBeInTheDocument()
    await advance(30_000)
    expect(fetchRunsFeed).toHaveBeenCalledTimes(3)
    expect(await screen.findByText('1/1 suites passed')).toBeInTheDocument()
  })

  it('does not poll a feed with no run awaiting suites', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(feed(makeRun(0)))
    renderWithProviders(<RunsFeedPage />)

    expect(await screen.findByText('1/1 suites passed')).toBeInTheDocument()
    await advance(120_000)
    expect(fetchRunsFeed).toHaveBeenCalledTimes(1)
  })

  // Every upload moves the run's timestamp, which used to be part of the row's identity: the
  // row was rebuilt and closed under the reader's eyes on the first poll that brought a suite.
  it('keeps an expanded row open across a poll that adds a suite', async () => {
    vi.mocked(fetchRunsFeed)
      .mockResolvedValueOnce(feed(makeRun(1, [[1, 3]])))
      .mockResolvedValue(
        feed(
          makeRun(
            undefined,
            [
              [1, 3],
              [2, 0],
            ],
            '2026-04-03T18:05:00Z',
          ),
        ),
      )
    vi.mocked(fetchRunFailures).mockResolvedValue(
      failuresOf(failure(1, 'a1'), failure(1, 'a2'), failure(1, 'a3')),
    )
    renderWithProviders(<RunsFeedPage />)

    await userEvent.setup().click(await screen.findByTestId('run-row-toggle'))
    expect(await screen.findByTestId('run-failures')).toBeInTheDocument()

    await advance(30_000)
    expect(await screen.findByText(/1\/2 suites failing/)).toBeInTheDocument()
    expect(screen.getByTestId('run-row-toggle')).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByTestId('run-failures')).toBeInTheDocument()
  })

  it('lists the failures of a suite that uploads and fails in a row that is open', async () => {
    vi.mocked(fetchRunsFeed)
      .mockResolvedValueOnce(feed(makeRun(1, [[1, 3]])))
      .mockResolvedValue(
        feed(
          makeRun(
            undefined,
            [
              [1, 3],
              [2, 2],
            ],
            '2026-04-03T18:05:00Z',
          ),
        ),
      )
    vi.mocked(fetchRunFailures)
      .mockResolvedValueOnce(failuresOf(failure(1, 'a1'), failure(1, 'a2'), failure(1, 'a3')))
      .mockResolvedValue(
        failuresOf(
          failure(1, 'a1'),
          failure(1, 'a2'),
          failure(1, 'a3'),
          failure(2, 'b1'),
          failure(2, 'b2'),
        ),
      )
    renderWithProviders(<RunsFeedPage />)

    await userEvent.setup().click(await screen.findByTestId('run-row-toggle'))
    expect(await screen.findByText('a1')).toBeInTheDocument()

    await advance(30_000)
    expect(await screen.findByText(/2\/2 suites failing/)).toBeInTheDocument()
    expect(fetchRunFailures).toHaveBeenCalledTimes(2)
    // The second suite's group is closed in the feed, but it is there, with its name.
    expect(
      await within(screen.getByTestId('run-failures')).findByText('ui-tests'),
    ).toBeInTheDocument()
  })
})
