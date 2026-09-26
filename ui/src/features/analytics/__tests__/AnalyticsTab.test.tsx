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
  })

  it('shows the empty state without trend or report data', async () => {
    renderTab()
    expect(await screen.findByText(/no report data yet/i)).toBeInTheDocument()
  })

  // Regression (d66811e): the stored branch is shared across projects, so one
  // this project lacks must not filter its analytics.
  it('filters by the stored branch only when the project has it', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      { id: 1, project_id: 1, name: 'main', is_default: true, created_at: '2024-01-01T00:00:00Z' },
    ])
    useUIStore.setState({ selectedBranch: 'gone' })
    renderTab()
    await screen.findByText(/no report data yet/i)

    act(() => {
      useUIStore.setState({ selectedBranch: 'main' })
    })

    await waitFor(() =>
      expect(analyticsApi.fetchTrends).toHaveBeenLastCalledWith('myproject', 100, 'main'),
    )
    expect(analyticsApi.fetchTrends).not.toHaveBeenCalledWith('myproject', 100, 'gone')
  })
})
