import { screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { PipelineRunsTab } from '../PipelineRunsTab'
import type { PaginatedResponse, PipelineRun } from '@/types/api'
import { useUIStore } from '@/store/ui'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRuns: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([]),
}))

// Must import after mock
import { fetchPipelineRuns } from '@/api/pipeline'

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
  suites: [
    {
      project_id: 1,
      slug: 'api-cloud',
      build_order: 5,
      pass_rate: 100,
      total: 42,
      failed: 0,
      duration_ms: 15000,
      status: 'passed',
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
}

describe('PipelineRunsTab', () => {
  beforeEach(() => {
    vi.mocked(fetchPipelineRuns).mockReset()
    useUIStore.setState({ selectedBranch: undefined })
  })

  it('renders pipeline run cards after data loads', async () => {
    vi.mocked(fetchPipelineRuns).mockResolvedValue(makeResponse([sampleRun]))

    renderWithProviders(
      <PipelineRunsTab projectId="parent" childIds={['api-cloud']} />,
    )

    await waitFor(() => {
      expect(screen.getByText('abc1234')).toBeInTheDocument()
    })

    expect(screen.getByText(/1\/1 suites passing/)).toBeInTheDocument()
  })

  it('shows empty state when no runs', async () => {
    vi.mocked(fetchPipelineRuns).mockResolvedValue(makeResponse([]))

    renderWithProviders(
      <PipelineRunsTab projectId="parent" childIds={['api-cloud']} />,
    )

    await waitFor(() => {
      expect(screen.getByText('No pipeline runs found')).toBeInTheDocument()
    })
  })

  it('displays parent project header with suite count', () => {
    vi.mocked(fetchPipelineRuns).mockResolvedValue(makeResponse([]))

    renderWithProviders(
      <PipelineRunsTab projectId="parent" childIds={['a', 'b', 'c']} />,
    )

    expect(screen.getByText('parent')).toBeInTheDocument()
    expect(screen.getByText(/3 suites/)).toBeInTheDocument()
  })
})
