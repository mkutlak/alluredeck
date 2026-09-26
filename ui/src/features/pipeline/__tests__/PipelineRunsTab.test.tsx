import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { PipelineRunsTab } from '../PipelineRunsTab'
import type { PaginatedResponse, PipelineRun } from '@/types/api'
import { useUIStore } from '@/store/ui'

vi.mock('@/api/pipeline', () => ({ fetchPipelineRuns: vi.fn() }))
vi.mock('@/api/branches', () => ({ fetchBranches: vi.fn() }))

import { fetchPipelineRuns } from '@/api/pipeline'
import { fetchBranches } from '@/api/branches'

function makeResponse(runs: PipelineRun[]): PaginatedResponse<PipelineRun[]> {
  return {
    data: runs,
    metadata: { message: 'ok' },
    pagination: { page: 1, per_page: 10, total: runs.length, total_pages: 1 },
  }
}

const sampleRun: PipelineRun = {
  commit_sha: 'abc1234',
  branch: 'main',
  ci_build_url: 'https://ci/1',
  timestamp: '2026-04-03T18:00:00Z',
  suites: [],
  aggregate: {
    suites_passed: 1,
    suites_total: 1,
    tests_passed: 42,
    tests_total: 42,
    pass_rate: 100,
    total_duration_ms: 15000,
  },
}

describe('PipelineRunsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(fetchBranches).mockResolvedValue([])
    useUIStore.setState({ selectedBranch: undefined })
  })

  it('renders pipeline run cards after data loads', async () => {
    vi.mocked(fetchPipelineRuns).mockResolvedValue(makeResponse([sampleRun]))
    renderWithProviders(<PipelineRunsTab projectId="parent" childIds={['api-cloud']} />)

    expect(await screen.findByText('abc1234')).toBeInTheDocument()
    expect(screen.getByText(/1\/1 suites passing/)).toBeInTheDocument()
  })

  it('shows the suite count and an empty state without runs', async () => {
    vi.mocked(fetchPipelineRuns).mockResolvedValue(makeResponse([]))
    renderWithProviders(<PipelineRunsTab projectId="parent" childIds={['a', 'b', 'c']} />)

    expect(await screen.findByText('No pipeline runs found')).toBeInTheDocument()
    expect(screen.getByText(/3 suites/)).toBeInTheDocument()
  })

  // Regression (d66811e): the stored branch is shared across projects, so one
  // this parent lacks must not filter its pipeline runs.
  it('filters by the stored branch only when the project has it', async () => {
    vi.mocked(fetchBranches).mockResolvedValue([
      { id: 1, project_id: 1, name: 'main', is_default: true, created_at: '2024-01-01T00:00:00Z' },
    ])
    vi.mocked(fetchPipelineRuns).mockResolvedValue(makeResponse([sampleRun]))
    useUIStore.setState({ selectedBranch: 'gone' })
    renderWithProviders(<PipelineRunsTab projectId="parent" childIds={['api-cloud']} />)
    await screen.findByText('abc1234')

    act(() => {
      useUIStore.setState({ selectedBranch: 'main' })
    })

    await waitFor(() =>
      expect(fetchPipelineRuns).toHaveBeenLastCalledWith('parent', 1, undefined, 'main'),
    )
    expect(fetchPipelineRuns).not.toHaveBeenCalledWith('parent', 1, undefined, 'gone')
  })
})
