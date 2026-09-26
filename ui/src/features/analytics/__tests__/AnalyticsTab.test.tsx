import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, screen, waitFor } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { AnalyticsTab } from '../AnalyticsTab'
import * as reportsApi from '@/api/reports'
import * as analyticsApi from '@/api/analytics'
import * as branchesApi from '@/api/branches'
import { useUIStore } from '@/store/ui'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
vi.mock('@/api/analytics')
vi.mock('@/api/branches')
mockApiClient()

function renderTab() {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/analytics', element: <AnalyticsTab /> }],
    { initialEntries: ['/projects/myproject/analytics'] },
  )
  return renderWithProviders(<></>, { router })
}

// The branch-filtered cards of the Quality and Distribution sections.
const cardFetches = [
  reportsApi.fetchLowPerformingTests,
  analyticsApi.fetchFlakyImpact,
  analyticsApi.fetchTopErrors,
  analyticsApi.fetchSuitePassRates,
  analyticsApi.fetchLabelBreakdown,
]

describe('AnalyticsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useUIStore.setState({ selectedBranch: undefined })
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([])
    vi.mocked(analyticsApi.fetchTrends).mockResolvedValue({
      status: [],
      pass_rate: [],
      duration: [],
      kpi: null,
    })
    vi.mocked(reportsApi.fetchReportHistory).mockResolvedValue({
      data: { project_id: 1, reports: [] },
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 1, total: 0, total_pages: 0 },
    })
    vi.mocked(reportsApi.fetchReportCategories).mockResolvedValue([])
    const noRows = { data: [], metadata: { message: 'ok' } }
    vi.mocked(analyticsApi.fetchTopErrors).mockResolvedValue(noRows)
    vi.mocked(analyticsApi.fetchSuitePassRates).mockResolvedValue(noRows)
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockResolvedValue(noRows)
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({ tests: [], builds: 20, total: 0 })
    vi.mocked(reportsApi.fetchLowPerformingTests).mockResolvedValue({
      tests: [],
      sort: 'duration',
      builds: 20,
      total: 0,
    })
  })

  it('shows the empty state without trend or report data', async () => {
    renderTab()
    expect(await screen.findByText(/no report data yet/i)).toBeInTheDocument()
  })

  // Regression (d66811e): the stored branch is shared across projects, so one
  // this project lacks must not filter its analytics — the trends nor any card.
  it('filters by the stored branch only when the project has it', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      { id: 1, project_id: 1, name: 'main', is_default: true, created_at: '2024-01-01T00:00:00Z' },
    ])
    vi.mocked(analyticsApi.fetchTrends).mockResolvedValue({
      status: [{ name: '#1', passed: 1, failed: 0, broken: 0, skipped: 0 }],
      pass_rate: [],
      duration: [],
      kpi: null,
    })
    // Distribution (suite + label cards) renders only with pie or category data.
    vi.mocked(reportsApi.fetchReportCategories).mockResolvedValue([
      {
        name: 'Product defects',
        matchedStatistic: { failed: 1, broken: 0, known: 0, unknown: 0, total: 1 },
      },
    ])
    useUIStore.setState({ selectedBranch: 'gone' })
    renderTab()
    await waitFor(() => {
      for (const fetchCard of cardFetches) expect(fetchCard).toHaveBeenCalled()
    })
    expect(analyticsApi.fetchTopErrors).toHaveBeenCalledWith('myproject', 20, 10, undefined)

    act(() => {
      useUIStore.setState({ selectedBranch: 'main' })
    })

    await waitFor(() =>
      expect(analyticsApi.fetchTopErrors).toHaveBeenLastCalledWith('myproject', 20, 10, 'main'),
    )
    expect(analyticsApi.fetchTrends).toHaveBeenLastCalledWith('myproject', 100, 'main')
    const allArgs = [analyticsApi.fetchTrends, ...cardFetches].flatMap((fn) =>
      vi.mocked(fn).mock.calls.flat(),
    )
    expect(allArgs).not.toContain('gone')
  })
})
