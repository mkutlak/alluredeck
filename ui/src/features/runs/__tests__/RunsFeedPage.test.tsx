import { act, screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunsFeedPage } from '../RunsFeedPage'
import { useUIStore } from '@/store/ui'
import type { PaginatedResponse, PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', parent_id: null, children: [1, 2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([
    { id: 1, project_id: 10, name: 'main', is_default: true, created_at: '2024-01-01T00:00:00Z' },
    {
      id: 2,
      project_id: 10,
      name: 'develop',
      is_default: false,
      created_at: '2024-01-01T00:00:00Z',
    },
  ]),
}))

vi.mock('@/api/builds', () => ({
  fetchBuildFailedTests: vi.fn().mockResolvedValue([]),
}))

import { fetchRunsFeed } from '@/api/pipeline'

function makeResponse(runs: PipelineRun[], overrides?: Partial<PaginatedResponse<PipelineRun[]>['pagination']>): PaginatedResponse<PipelineRun[]> {
  return {
    data: runs,
    metadata: { message: 'ok' },
    pagination: { page: 1, per_page: 10, total: runs.length, total_pages: 1, ...overrides },
  }
}

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    commit_sha: 'abc1234',
    branch: 'main',
    ci_build_url: 'https://ci/1',
    timestamp: '2026-04-03T18:00:00Z',
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

describe('RunsFeedPage', () => {
  beforeEach(() => {
    vi.mocked(fetchRunsFeed).mockReset()
    useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
  })

  it('has data-testid="runs-feed"', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([]))
    renderWithProviders(<RunsFeedPage />)
    expect(screen.getByTestId('runs-feed')).toBeInTheDocument()
  })

  it('renders a row per run once data loads', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([makeRun()]))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(screen.getAllByTestId('run-row')).toHaveLength(1)
    })
  })

  it('shows an empty state with a hint and a link to /projects', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([]))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(screen.getByText(/CI metadata/i)).toBeInTheDocument()
    })
    const link = screen.getByRole('link', { name: /projects/i })
    expect(link).toHaveAttribute('href', '/projects')
  })

  it('passes selected group_id filters through to fetchRunsFeed', async () => {
    useUIStore.setState({ runsFeedGroupIds: [10] })
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([]))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, undefined, [10])
    })
  })

  it('calls fetchRunsFeed with undefined groupIds when none are selected', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([]))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, undefined, undefined)
    })
  })

  it('resets to page 1 when the branch filter changes', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(
      makeResponse([makeRun()], { page: 2, total_pages: 3 }),
    )
    useUIStore.setState({ selectedBranch: 'main' })
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, 'main', undefined)
    })

    vi.mocked(fetchRunsFeed).mockClear()
    act(() => {
      useUIStore.setState({ selectedBranch: 'develop' })
    })

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, 'develop', undefined)
    })
  })

  it('sends no branch param when the stored branch is absent from the available branches', async () => {
    useUIStore.setState({ selectedBranch: 'nonexistent' })
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([]))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, undefined, undefined)
    })
  })

  it('resets to page 1 when the group filter changes', async () => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([makeRun()]))
    renderWithProviders(<RunsFeedPage />)

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, undefined, undefined)
    })

    vi.mocked(fetchRunsFeed).mockClear()
    act(() => {
      useUIStore.getState().setRunsFeedGroupIds([10])
    })

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenCalledWith(1, undefined, undefined, [10])
    })
  })
})
