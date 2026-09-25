import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { OverviewTab } from '../OverviewTab'
import * as reportsApi from '@/api/reports'
import * as branchesApi from '@/api/branches'
import * as projectsApi from '@/api/projects'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([]),
}))
vi.mock('@/api/projects')
mockApiClient()

function makeReport(id: string, isLatest = false, passed = 10, total = 10) {
  return {
    report_id: id,
    is_latest: isLatest,
    generated_at: '2024-01-15T10:00:00Z',
    duration_ms: 5000,
    statistic: { passed, failed: total - passed, broken: 0, skipped: 0, unknown: 0, total },
  }
}

function makePaginated(
  reports: ReturnType<typeof makeReport>[],
  pagination: { page: number; per_page: number; total: number; total_pages: number },
) {
  return {
    data: { project_id: 1, reports },
    metadata: { message: 'ok' },
    pagination,
  }
}

function renderTab() {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: ['viewer'],
    username: 'viewer',
    expiresAt: Date.now() + 3_600_000,
  })

  // The route param below is the slug; the index maps it to numeric project_id 7.
  vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
    data: [{ project_id: 7, slug: 'test-project' }],
    metadata: { message: 'ok' },
  })
  vi.mocked(reportsApi.fetchReportKnownFailures).mockResolvedValue({
    known_failures: [],
    new_failures: [],
    adjusted_stats: { known_count: 0, new_count: 0, total_count: 0 },
  })
  vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([])
  vi.mocked(reportsApi.fetchReportCategories).mockResolvedValue([])
  vi.mocked(reportsApi.fetchReportStability).mockResolvedValue({
    flaky_tests: [],
    new_failed: [],
    new_passed: [],
    summary: {
      flaky_count: 0,
      retried_count: 0,
      new_failed_count: 0,
      new_passed_count: 0,
      total: 0,
    },
  })

  const router = createMemoryRouter([{ path: '/projects/:id', element: <OverviewTab /> }], {
    initialEntries: ['/projects/test-project'],
  })

  return renderWithProviders(<></>, { router })
}

describe('OverviewTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useUIStore.setState({ selectedBranch: undefined, reportsPerPage: 20 })
  })

  it('pages forward and back through the report history', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockImplementation((_pid, page = 1, perPage = 20) =>
      Promise.resolve(
        makePaginated([makeReport('latest', true), makeReport(`page${page}-report`)], {
          page,
          per_page: perPage,
          total: 25,
          total_pages: 2,
        }),
      ),
    )
    renderTab()

    expect(await screen.findByText('#page1-report')).toBeInTheDocument()
    expect(screen.queryByText('#latest')).not.toBeInTheDocument()
    const pager = within(screen.getByRole('navigation', { name: /pagination/i }))
    expect(pager.getByText(/page 1 of 2/i)).toBeInTheDocument()
    expect(pager.getByRole('button', { name: /previous/i })).toBeDisabled()

    await user.click(pager.getByRole('button', { name: /next/i }))
    expect(await screen.findByText('#page2-report')).toBeInTheDocument()
    expect(reportsApi.fetchReportHistory).toHaveBeenCalledWith('test-project', 2, 20, undefined)
    expect(pager.getByText(/page 2 of 2/i)).toBeInTheDocument()
    expect(pager.getByRole('button', { name: /next/i })).toBeDisabled()

    await user.click(pager.getByRole('button', { name: /previous/i }))
    expect(await screen.findByText('#page1-report')).toBeInTheDocument()
  })

  // Pagination stays visible for a single page so rows-per-page remains reachable.
  it('shows pagination controls (with nav disabled) when total_pages <= 1', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true), makeReport('1')], {
        page: 1,
        per_page: 20,
        total: 1,
        total_pages: 1,
      }),
    )
    renderTab()

    expect(await screen.findByText('#1')).toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: /pagination/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
  })

  it('shows the empty state when the page holds only the "latest" alias', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true)], {
        page: 1,
        per_page: 20,
        total: 0,
        total_pages: 0,
      }),
    )
    renderTab()

    expect(await screen.findByText(/no reports yet/i)).toBeInTheDocument()
  })

  // Links use the numeric project_id even when the route param is a slug —
  // slugs collide for same-name children under different parents (ui/CLAUDE.md).
  it('links reports and two selected builds by numeric project id, blocks a third, and clears', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41'), makeReport('40')], {
        page: 1,
        per_page: 20,
        total: 3,
        total_pages: 1,
      }),
    )
    renderTab()
    const checkbox = (id: string) => screen.getByRole('checkbox', { name: `Select report #${id}` })

    const reportLink = await screen.findByRole('link', { name: '#41' })
    expect(reportLink).toHaveAttribute('href', '/projects/7/reports/41')
    expect(
      within(reportLink.closest('tr')!).getByRole('link', { name: /open in new tab/i }),
    ).toHaveAttribute('href', expect.stringMatching(/\/projects\/7\/reports\/41\/index\.html$/))

    await user.click(checkbox('41'))
    await user.click(checkbox('40'))

    expect(screen.getByRole('link', { name: /compare selected/i })).toHaveAttribute(
      'href',
      '/projects/7/compare?a=41&b=40',
    )
    expect(checkbox('42')).toBeDisabled()

    await user.click(screen.getByRole('button', { name: /clear/i }))

    expect(screen.queryByRole('link', { name: /compare selected/i })).not.toBeInTheDocument()
    expect(checkbox('41')).not.toBeChecked()
    expect(checkbox('40')).not.toBeChecked()
  })

  // The API prepends the "latest" alias unfiltered by branch, so the header chips
  // come from a separate 1-entry branch-filtered query to agree with the table.
  // Regression (d66811e): the stored branch is shared across projects, so one this
  // project lacks must neither filter its history nor show a Branch chip.
  it.each([
    { stored: 'main', branch: 'main' },
    { stored: 'missing-branch', branch: undefined },
  ])(
    'derives header chips from the branch-filtered newest build (stored branch $stored)',
    async ({ stored, branch }) => {
      useUIStore.setState({ selectedBranch: stored })
      vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
        {
          id: 1,
          project_id: 1,
          name: 'main',
          is_default: true,
          created_at: '2024-01-01T00:00:00Z',
        },
      ])
      vi.mocked(reportsApi.fetchReportHistory).mockImplementation((_pid, _page, perPage) =>
        Promise.resolve(
          perPage === 1
            ? makePaginated([makeReport('42', false, 10, 10)], {
                page: 1,
                per_page: 1,
                total: 5,
                total_pages: 5,
              })
            : makePaginated([makeReport('latest', true, 1, 10), makeReport('41', false, 5, 10)], {
                page: 1,
                per_page: 20,
                total: 1,
                total_pages: 1,
              }),
        ),
      )
      renderTab()

      // The effective branch is resolved against the project's branch list.
      await waitFor(() =>
        expect(screen.getByRole('combobox', { name: /filter by branch/i })).toBeEnabled(),
      )
      await waitFor(() => {
        const chipRow = within(screen.getByText(/pass rate: 100%/i).parentElement!)
        expect(chipRow.queryByText(/^Branch:/)?.textContent).toBe(branch && `Branch: ${branch}`)
      })
      expect(screen.queryByText(/pass rate: (10|50)%/i)).not.toBeInTheDocument()
      expect(reportsApi.fetchReportHistory).toHaveBeenLastCalledWith(
        'test-project',
        1,
        expect.any(Number),
        branch,
      )
    },
  )
})
