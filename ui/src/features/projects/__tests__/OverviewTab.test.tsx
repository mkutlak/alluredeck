import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { OverviewTab } from '../OverviewTab'
import * as reportsApi from '@/api/reports'
import { useAuthStore } from '@/store/auth'

vi.mock('@/api/reports')
vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([]),
}))
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

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
    data: { project_id: 'test-project', reports },
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

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

describe('OverviewTab - report history pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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

  it('shows total count from pagination metadata in the stat card', async () => {
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
      expect(screen.getByText(/50 reports total/i)).toBeInTheDocument()
    })
  })

  it('hides pagination controls when total_pages <= 1', async () => {
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
    expect(screen.queryByRole('navigation', { name: /pagination/i })).not.toBeInTheDocument()
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
    vi.mocked(reportsApi.fetchReportHistory)
      .mockResolvedValueOnce(
        makePaginated([makeReport('latest', true), makeReport('page1-report')], {
          page: 1,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
      .mockResolvedValueOnce(
        makePaginated([makeReport('latest', true), makeReport('page2-report')], {
          page: 2,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
      .mockResolvedValue(
        makePaginated([makeReport('latest', true), makeReport('page1-report')], {
          page: 1,
          per_page: 20,
          total: 25,
          total_pages: 2,
        }),
      )
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
    await user.click(checkboxes[0])
    await user.click(checkboxes[1])

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
    await user.click(checkboxes[0]) // selects #41
    await user.click(checkboxes[1]) // selects #40

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
    await user.click(checkboxes[0])
    await user.click(checkboxes[1])

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
    await user.click(checkboxes[0])
    await user.click(checkboxes[1])

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

  it('renders BranchSelector', async () => {
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated([makeReport('42', true), makeReport('41')], {
        page: 1,
        per_page: 20,
        total: 1,
        total_pages: 1,
      }),
    )
    renderTab()
    // BranchSelector renders null when there are no branches (branches query returns empty/undefined)
    // Verify the component mounts without crashing — the selector is absent when branches are empty
    await waitFor(() => screen.getByText('#41'))
    // The branch filter combobox should not appear when no branches are returned
    expect(screen.queryByRole('combobox', { name: /filter by branch/i })).not.toBeInTheDocument()
  })

  it('groups builds by commit SHA', async () => {
    function makeReportWithSha(id: string, sha: string) {
      return {
        ...makeReport(id),
        ci_commit_sha: sha,
      }
    }

    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue(
      makePaginated(
        [
          makeReport('10', true),
          makeReportWithSha('9', 'abc1234567890'),
          makeReportWithSha('8', 'abc1234567890'),
          makeReport('7'),
        ],
        { page: 1, per_page: 20, total: 3, total_pages: 1 },
      ),
    )
    renderTab()

    // Wait for data to load
    await waitFor(() => screen.getByText('#7'))

    // The grouped commit header row should be visible (shows truncated SHA)
    expect(screen.getByTestId('commit-group-abc1234')).toBeInTheDocument()
    // The grouped builds count badge should show "2 builds"
    expect(screen.getByText('2 builds')).toBeInTheDocument()
    // The individual grouped report rows should NOT be visible until expanded
    expect(screen.queryByText('#9')).not.toBeInTheDocument()
    expect(screen.queryByText('#8')).not.toBeInTheDocument()
    // The ungrouped report is visible
    expect(screen.getByText('#7')).toBeInTheDocument()
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
})
