import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

function makeResponse(
  runs: PipelineRun[],
  overrides?: Partial<PaginatedResponse<PipelineRun[]>['pagination']>,
): PaginatedResponse<PipelineRun[]> {
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

// The branch select is enabled once the groups' branches have loaded, i.e. once
// the effective branch filter is settled.
async function waitForBranches() {
  await waitFor(() => {
    expect(screen.getByRole('combobox', { name: /filter by branch/i })).toBeEnabled()
  })
}

describe('RunsFeedPage', () => {
  beforeEach(() => {
    vi.mocked(fetchRunsFeed).mockReset()
    useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
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

    expect(await screen.findByText(/CI metadata/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /projects/i })).toHaveAttribute('href', '/projects')
  })

  it.each([
    { name: 'no filters', branch: undefined, groups: [], want: [undefined, undefined] },
    { name: 'a known stored branch', branch: 'main', groups: [], want: ['main', undefined] },
    // The stored branch is shared across pages; one no group has must not filter the feed.
    {
      name: 'an unknown stored branch',
      branch: 'nonexistent',
      groups: [],
      want: [undefined, undefined],
    },
    { name: 'selected groups', branch: undefined, groups: [10], want: [undefined, [10]] },
  ])('requests the feed for $name', async ({ branch, groups, want }) => {
    useUIStore.setState({ selectedBranch: branch, runsFeedGroupIds: groups })
    vi.mocked(fetchRunsFeed).mockResolvedValue(makeResponse([]))
    renderWithProviders(<RunsFeedPage />)

    await waitForBranches()
    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenLastCalledWith(1, undefined, ...want)
    })
  })

  it.each([
    {
      name: 'branch',
      change: () => useUIStore.setState({ selectedBranch: 'develop' }),
      want: ['develop', undefined],
    },
    {
      name: 'group',
      change: () => useUIStore.getState().setRunsFeedGroupIds([10]),
      want: [undefined, [10]],
    },
  ])('resets to page 1 when the $name filter changes', async ({ change, want }) => {
    const user = userEvent.setup()
    vi.mocked(fetchRunsFeed).mockImplementation((page = 1) =>
      Promise.resolve(makeResponse([makeRun()], { page, total_pages: 3 })),
    )
    renderWithProviders(<RunsFeedPage />)
    await waitForBranches()

    await user.click(await screen.findByRole('button', { name: /next/i }))
    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenLastCalledWith(2, undefined, undefined, undefined)
    })

    act(() => {
      change()
    })

    await waitFor(() => {
      expect(fetchRunsFeed).toHaveBeenLastCalledWith(1, undefined, ...want)
    })
  })
})
