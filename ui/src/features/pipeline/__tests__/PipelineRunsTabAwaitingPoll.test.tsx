import { act, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { PipelineRunsTab } from '../PipelineRunsTab'
import { useUIStore } from '@/store/ui'
import type { PaginatedResponse, PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({ fetchPipelineRuns: vi.fn() }))
vi.mock('@/api/branches', () => ({ fetchBranches: vi.fn() }))

import { fetchPipelineRuns } from '@/api/pipeline'
import { fetchBranches } from '@/api/branches'

function makeRun(awaiting?: number): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    suites: [
      {
        project_id: 1,
        slug: 'api-cloud',
        build_number: 5,
        build_id: 105,
        pass_rate: 100,
        total: 100,
        failed: 0,
        duration_ms: 15000,
        status: 'passed',
      },
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 100,
      tests_total: 100,
      pass_rate: 100,
      total_duration_ms: 15000,
      awaiting_suites: awaiting,
    },
  }
}

const reply = (run: PipelineRun): PaginatedResponse<PipelineRun[]> => ({
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

describe('PipelineRunsTab polling for a run that is still uploading', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    vi.mocked(fetchPipelineRuns).mockReset()
    vi.mocked(fetchBranches).mockResolvedValue([])
    useUIStore.setState({ selectedBranch: undefined })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('refetches every 30 s while a run awaits suites, then stops once none does', async () => {
    vi.mocked(fetchPipelineRuns)
      .mockResolvedValueOnce(reply(makeRun(2)))
      .mockResolvedValue(reply(makeRun()))
    renderWithProviders(<PipelineRunsTab projectId="parent" childIds={['api-cloud']} />)

    expect(await screen.findByText('awaiting 2 suites')).toBeInTheDocument()
    expect(fetchPipelineRuns).toHaveBeenCalledTimes(1)

    await advance(29_000)
    expect(fetchPipelineRuns).toHaveBeenCalledTimes(1)
    await advance(1_000)
    expect(fetchPipelineRuns).toHaveBeenCalledTimes(2)
    expect(await screen.findByText('1/1 suites passing')).toBeInTheDocument()

    await advance(120_000)
    expect(fetchPipelineRuns).toHaveBeenCalledTimes(2)
  })

  it('does not poll runs that are not awaiting suites', async () => {
    vi.mocked(fetchPipelineRuns).mockResolvedValue(reply(makeRun(0)))
    renderWithProviders(<PipelineRunsTab projectId="parent" childIds={['api-cloud']} />)

    expect(await screen.findByText('1/1 suites passing')).toBeInTheDocument()
    await advance(120_000)
    expect(fetchPipelineRuns).toHaveBeenCalledTimes(1)
  })
})
