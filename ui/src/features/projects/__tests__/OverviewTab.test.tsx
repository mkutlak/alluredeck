import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { OverviewTab } from '../OverviewTab'
import * as reportsApi from '@/api/reports'
import * as branchesApi from '@/api/branches'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([]),
}))
mockApiClient()

function makeReport(id: string, isLatest = false) {
  return {
    report_id: id,
    is_latest: isLatest,
    generated_at: '2024-01-15T10:00:00Z',
    duration_ms: 5000,
    statistic: { passed: 10, failed: 2, broken: 0, skipped: 1, unknown: 0, total: 13 },
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

function renderTab(isAdminUser = false) {
  useAuthStore.setState({
    isAuthenticated: true,
    roles: isAdminUser ? ['admin'] : ['viewer'],
    username: isAdminUser ? 'admin' : 'viewer',
    expiresAt: Date.now() + 3_600_000,
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

describe('OverviewTab - report history pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useUIStore.setState({ selectedBranch: undefined, reportsPerPage: 20 })
  })

  it('filters the synthetic "latest" alias out of the history table', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true), makeReport('41'), makeReport('40')], {
        page: 1,
        per_page: 20,
        total: 2,
        total_pages: 1,
      }),
    )
    renderTab()
    await waitFor(() => {
      expect(screen.queryByText('#latest')).not.toBeInTheDocument()
      expect(screen.getByText('#41')).toBeInTheDocument()
      expect(screen.getByText('#40')).toBeInTheDocument()
    })
  })

  it('shows empty state when only the synthetic "latest" alias is returned', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true)], {
        page: 1,
        per_page: 20,
        total: 0,
        total_pages: 0,
      }),
    )
    renderTab()
    await waitFor(() => {
      expect(screen.getByText(/no reports yet/i)).toBeInTheDocument()
    })
  })

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
    await waitFor(() => screen.getByText('#1'))
    expect(screen.getByRole('navigation', { name: /pagination/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
  })

  it('shows pagination controls when total_pages > 1', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true), makeReport('20'), makeReport('19')], {
        page: 1,
        per_page: 20,
        total: 50,
        total_pages: 3,
      }),
    )
    renderTab()
    await waitFor(() => {
      expect(screen.getByRole('navigation', { name: /pagination/i })).toBeInTheDocument()
    })
  })

  it('shows page info text', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true), makeReport('20')], {
        page: 1,
        per_page: 20,
        total: 50,
        total_pages: 3,
      }),
    )
    renderTab()
    await waitFor(() => {
      expect(screen.getByText(/page 1 of 3/i)).toBeInTheDocument()
    })
  })

  it('next button fetches the next page', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory)
      .mockResolvedValueOnce(
        makePaginated([makeReport('latest', true), makeReport('20')], {
          page: 1,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
      .mockResolvedValue(
        makePaginated([makeReport('latest', true), makeReport('5')], {
          page: 2,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
    renderTab()
    await waitFor(() => screen.getByText('#20'))
    await user.click(screen.getByRole('button', { name: /next/i }))
    await waitFor(() => {
      expect(screen.getByText('#5')).toBeInTheDocument()
    })
    expect(reportsApi.fetchReportHistory).toHaveBeenCalledWith('test-project', 2, 20, undefined)
  })

  it('previous button shows prior page after navigating forward', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockImplementation((_pid, page, perPage) => {
      if (perPage === 1) {
        // Header stat chips query — not part of this pagination flow.
        return Promise.resolve(
          makePaginated([makeReport('page1-report', true)], {
            page: 1,
            per_page: 1,
            total: 25,
            total_pages: 25,
          }),
        )
      }
      if (page === 2) {
        return Promise.resolve(
          makePaginated([makeReport('latest', true), makeReport('page2-report')], {
            page: 2,
            per_page: 20,
            total: 25,
            total_pages: 2,
          }),
        )
      }
      return Promise.resolve(
        makePaginated([makeReport('latest', true), makeReport('page1-report')], {
          page: 1,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
    })
    renderTab()
    await waitFor(() => screen.getByText('#page1-report'))
    await user.click(screen.getByRole('button', { name: /next/i }))
    await waitFor(() => screen.getByText('#page2-report'))
    await user.click(screen.getByRole('button', { name: /previous/i }))
    await waitFor(() => {
      expect(screen.getByText('#page1-report')).toBeInTheDocument()
    })
  })

  it('disables previous button on the first page', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true), makeReport('20')], {
        page: 1,
        per_page: 20,
        total: 25,
        total_pages: 2,
      }),
    )
    renderTab()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled()
    })
  })

  it('renders checkboxes in table rows', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41'), makeReport('40')], {
        page: 1,
        per_page: 20,
        total: 2,
        total_pages: 1,
      }),
    )
    renderTab()
    await waitFor(() => screen.getByText('#41'))

    // Each non-latest report row should have a checkbox
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes.length).toBeGreaterThanOrEqual(2)
  })

  it('selecting 2 builds shows compare button and link', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41'), makeReport('40')], {
        page: 1,
        per_page: 20,
        total: 2,
        total_pages: 1,
      }),
    )
    renderTab()
    await waitFor(() => screen.getByText('#41'))

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]!)
    await user.click(checkboxes[1]!)

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /compare selected/i })).toBeInTheDocument()
    })
  })

  it('compare link contains correct build params', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41'), makeReport('40')], {
        page: 1,
        per_page: 20,
        total: 2,
        total_pages: 1,
      }),
    )
    renderTab()
    await waitFor(() => screen.getByText('#41'))

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]!) // selects #41
    await user.click(checkboxes[1]!) // selects #40

    await waitFor(() => {
      const link = screen.getByRole('link', { name: /compare selected/i })
      const href = link.getAttribute('href') ?? ''
      expect(href).toMatch(/compare/)
      expect(href).toMatch(/a=/)
      expect(href).toMatch(/b=/)
    })
  })

  it('cannot select more than 2 builds', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated(
        [makeReport('42', true), makeReport('41'), makeReport('40'), makeReport('39')],
        { page: 1, per_page: 20, total: 3, total_pages: 1 },
      ),
    )
    renderTab()
    await waitFor(() => screen.getByText('#41'))

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]!)
    await user.click(checkboxes[1]!)

    // Third checkbox should be disabled when 2 are already selected
    await waitFor(() => {
      expect(checkboxes[2]).toBeDisabled()
    })
  })

  it('clear button resets selection', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41'), makeReport('40')], {
        page: 1,
        per_page: 20,
        total: 2,
        total_pages: 1,
      }),
    )
    renderTab()
    await waitFor(() => screen.getByText('#41'))

    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0]!)
    await user.click(checkboxes[1]!)

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /compare selected/i })).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: /clear/i }))

    await waitFor(() => {
      expect(screen.queryByRole('link', { name: /compare selected/i })).not.toBeInTheDocument()
    })
    expect(checkboxes[0]).not.toBeChecked()
    expect(checkboxes[1]).not.toBeChecked()
  })

  it('renders the branch filter in the filter row', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41')], {
        page: 1,
        per_page: 20,
        total: 1,
        total_pages: 1,
      }),
    )
    renderTab()
    // BranchSelect renders null when there are no branches (branches query returns empty/undefined)
    // Verify the component mounts without crashing — the selector is absent when branches are empty
    await waitFor(() => screen.getByText('#41'))
    // The branch filter combobox should not appear when no branches are returned
    expect(screen.queryByRole('combobox', { name: /filter by branch/i })).not.toBeInTheDocument()
  })

  it('disables next button on the last page', async () => {
    const user = userEvent.setup()
    vi.mocked(reportsApi.fetchReportHistory)
      .mockResolvedValueOnce(
        makePaginated([makeReport('latest', true), makeReport('20')], {
          page: 1,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
      .mockResolvedValue(
        makePaginated([makeReport('latest', true), makeReport('5')], {
          page: 2,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
    renderTab()
    await waitFor(() => screen.getByText('#20'))
    await user.click(screen.getByRole('button', { name: /next/i }))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
    })
  })

  it("falls back to undefined when stored branch is not in project's branch list", async () => {
    useUIStore.setState({ selectedBranch: 'missing-branch' })
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      {
        id: 1,
        project_id: 1,
        name: 'master',
        is_default: true,
        created_at: '2024-01-01T00:00:00Z',
      },
    ])
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('latest', true), makeReport('1')], {
        page: 1,
        per_page: 20,
        total: 1,
        total_pages: 1,
      }),
    )
    renderTab()
    await waitFor(() => {
      expect(reportsApi.fetchReportHistory).toHaveBeenCalledWith('test-project', 1, 20, undefined)
    })
  })
})

describe('OverviewTab - header stat chips branch consistency', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useUIStore.setState({ selectedBranch: undefined, reportsPerPage: 20 })
  })

  function makeReportWithStats(id: string, passed: number, total: number, isLatest = false) {
    return {
      report_id: id,
      is_latest: isLatest,
      generated_at: '2024-01-15T10:00:00Z',
      duration_ms: 5000,
      statistic: { passed, failed: total - passed, broken: 0, skipped: 0, unknown: 0, total },
    }
  }

  it('derives header chips from the newest branch-filtered numbered build, not the unfiltered "latest" alias', async () => {
    // The synthetic "latest" entry (unfiltered by branch) has a low pass rate,
    // while the newest branch-filtered numbered build has a high pass rate.
    vi.mocked(reportsApi.fetchReportHistory).mockImplementation((_pid, _page, perPage) => {
      if (perPage === 1) {
        // Chip query: branch-filtered, newest numbered build only
        return Promise.resolve(
          makePaginated([makeReportWithStats('42', 10, 10)], {
            page: 1,
            per_page: 1,
            total: 5,
            total_pages: 5,
          }),
        )
      }
      // Table query: includes the unfiltered synthetic "latest" alias
      return Promise.resolve(
        makePaginated(
          [makeReportWithStats('latest', 1, 10, true), makeReportWithStats('42', 10, 10)],
          { page: 1, per_page: 20, total: 1, total_pages: 1 },
        ),
      )
    })
    renderTab()

    await waitFor(() => {
      expect(screen.getByText(/pass rate: 100%/i)).toBeInTheDocument()
    })
    expect(screen.queryByText(/pass rate: 10%/i)).not.toBeInTheDocument()
  })

  it('shows a Branch chip when a branch filter is active', async () => {
    useUIStore.setState({ selectedBranch: 'main' })
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      {
        id: 1,
        project_id: 1,
        name: 'main',
        is_default: true,
        created_at: '2024-01-01T00:00:00Z',
      },
    ])
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReportWithStats('42', 10, 10, true)], {
        page: 1,
        per_page: 20,
        total: 1,
        total_pages: 1,
      }),
    )
    renderTab()

    await waitFor(() => {
      expect(screen.getByText('Branch: main')).toBeInTheDocument()
    })
  })

  it('does not show a Branch chip when no branch filter is active', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReportWithStats('42', 10, 10, true)], {
        page: 1,
        per_page: 20,
        total: 1,
        total_pages: 1,
      }),
    )
    renderTab()

    await waitFor(() => {
      expect(screen.getByText(/pass rate: 100%/i)).toBeInTheDocument()
    })
    // The stat-chip Badge renders "Branch: <name>"; the branch filter's own
    // "Branch:" label (no value) is a separate element and is expected to be present.
    expect(screen.queryByText(/^Branch: \S/)).not.toBeInTheDocument()
  })
})
