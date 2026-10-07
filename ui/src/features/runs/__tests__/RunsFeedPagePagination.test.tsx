import { screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunsFeedPage } from '../RunsFeedPage'
import { useUIStore } from '@/store/ui'
import type { PaginatedResponse, PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunsFeed: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunFailures: vi.fn().mockResolvedValue({
    data: [],
    metadata: { message: 'ok', truncated: false },
  }),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', parent_id: null, children: [1, 2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([]),
}))

import { fetchRunsFeed } from '@/api/pipeline'

function makeResponse(totalPages: number): PaginatedResponse<PipelineRun[]> {
  const run: PipelineRun = {
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
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
  return {
    data: [run],
    metadata: { message: 'ok' },
    pagination: { page: 1, per_page: 10, total: 1, total_pages: totalPages },
  }
}

describe('RunsFeedPage pagination', () => {
  beforeEach(() => {
    vi.mocked(fetchRunsFeed).mockReset()
    useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
  })

  // "Page 1 of 1" with two disabled buttons is noise: nothing to page through.
  it('is hidden when everything fits on one page', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse(1))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(screen.getAllByTestId('run-row')).toHaveLength(1)
    })
    expect(screen.queryByText(/page 1 of 1/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /previous/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument()
  })

  it('shows the page count and both buttons when there is more than one page', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse(3))
    renderWithProviders(<RunsFeedPage />)

    expect(await screen.findByText('Page 1 of 3')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /next/i })).toBeEnabled()
  })
})
