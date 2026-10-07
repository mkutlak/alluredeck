import { act, screen, waitFor } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunPage } from '../RunPage'
import type { PipelineRun, RunFailure, RunFailuresResponse } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRun: vi.fn(),
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', display_name: 'Acme Payments', children: [1, 2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi
    .fn()
    .mockResolvedValue({ data: { llm_enabled: false }, metadata: { message: 'ok' } }),
}))

import { fetchPipelineRun, fetchRunFailures } from '@/api/pipeline'

// Suites as [project id, failed tests]; 1 and 2 pass unless told otherwise.
function makeRun(
  awaiting?: number,
  suites: Array<[number, number]> = [
    [1, 0],
    [2, 0],
  ],
): PipelineRun {
  return {
    pipeline_id: '48213',
    commit_sha: 'a1b2c3d4e5f6',
    branch: 'feature/sepa',
    timestamp: '2026-10-07T09:12:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: suites.map(([id, failed]) => ({
      project_id: id,
      slug: `suite-${id}`,
      build_number: 10 + id,
      build_id: 100 + id,
      pass_rate: 100,
      total: 100,
      passed: 100 - failed,
      failed,
      skipped: 0,
      duration_ms: 42000,
      status: failed > 0 ? 'degraded' : 'passed',
    })),
    aggregate: {
      suites_passed: suites.filter(([, failed]) => failed === 0).length,
      suites_total: suites.length,
      tests_passed: 200,
      tests_total: 200,
      tests_skipped: 0,
      pass_rate: 100,
      total_duration_ms: 213000,
      awaiting_suites: awaiting,
    },
  }
}

function failure(project_id: number, test_name: string): RunFailure {
  return {
    project_id,
    slug: `suite-${project_id}`,
    build_id: 100 + project_id,
    build_number: 10 + project_id,
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

const failuresOf = (...rows: RunFailure[]): RunFailuresResponse => ({
  data: rows,
  metadata: { message: 'ok', truncated: false },
})

const reply = (run: PipelineRun) => ({ data: run, metadata: { message: 'ok' } })
const summary = () => screen.getByTestId('run-page-summary')

function renderRun() {
  const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
    initialEntries: ['/runs/10/48213'],
  })
  return renderWithProviders(<></>, { router })
}

// Only the interval clock is faked. Testing Library's waitFor polls with setInterval too,
// so findBy* resolves on its first check or when the DOM changes (MutationObserver), which
// is all these waits need: each one is for something the render is about to show.
const advance = (ms: number) =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })

describe('RunPage polling for a run that is still uploading', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    vi.mocked(fetchPipelineRun).mockReset()
    vi.mocked(fetchRunFailures).mockReset()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('refetches every 30 s while the run awaits suites, then stops once it does not', async () => {
    vi.mocked(fetchPipelineRun)
      .mockResolvedValueOnce(reply(makeRun(2)))
      .mockResolvedValue(reply(makeRun()))
    renderRun()

    await screen.findByRole('heading', { level: 1 })
    expect(summary()).toHaveTextContent(/^awaiting 2 suites/)
    expect(fetchPipelineRun).toHaveBeenCalledTimes(1)

    await advance(29_000)
    expect(fetchPipelineRun).toHaveBeenCalledTimes(1)
    await advance(1_000)
    expect(fetchPipelineRun).toHaveBeenCalledTimes(2)
    expect(await screen.findByText(/^2\/2 suites passed/)).toBeInTheDocument()

    await advance(120_000)
    expect(fetchPipelineRun).toHaveBeenCalledTimes(2)
  })

  it('does not poll a run that is not awaiting suites', async () => {
    vi.mocked(fetchPipelineRun).mockResolvedValue(reply(makeRun(0)))
    renderRun()

    await screen.findByRole('heading', { level: 1 })
    await advance(120_000)
    expect(fetchPipelineRun).toHaveBeenCalledTimes(1)
  })

  // The poll can bring the last suite, and it can fail. Its failures were not there when the
  // list was fetched, so the list has to be fetched again rather than wait out its cache.
  it('lists the failures of a suite that uploads and fails, without a reload', async () => {
    vi.mocked(fetchPipelineRun)
      .mockResolvedValueOnce(reply(makeRun(1, [[1, 2]])))
      .mockResolvedValue(
        reply(
          makeRun(undefined, [
            [1, 2],
            [2, 1],
          ]),
        ),
      )
    let release!: (response: RunFailuresResponse) => void
    vi.mocked(fetchRunFailures)
      .mockResolvedValueOnce(failuresOf(failure(1, 'a1'), failure(1, 'a2')))
      .mockReturnValueOnce(new Promise((resolve) => (release = resolve)))
    renderRun()

    expect(await screen.findByText('a1')).toBeInTheDocument()
    expect(fetchRunFailures).toHaveBeenCalledTimes(1)

    await advance(30_000)
    await waitFor(() => expect(summary()).toHaveTextContent(/^2\/2 suites failing/))
    expect(fetchRunFailures).toHaveBeenCalledTimes(2)
    // The list it has stays up while the new one loads: no flash to a skeleton.
    expect(screen.getByText('a1')).toBeInTheDocument()

    await act(async () => {
      release(failuresOf(failure(1, 'a1'), failure(1, 'a2'), failure(2, 'b1')))
    })
    expect(await screen.findByText('b1')).toBeInTheDocument()
    expect(screen.queryByText(/no failing tests listed/i)).not.toBeInTheDocument()
  })
})
