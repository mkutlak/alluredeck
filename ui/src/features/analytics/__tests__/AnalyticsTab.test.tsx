import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { AnalyticsTab } from '../AnalyticsTab'
import * as reportsApi from '@/api/reports'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
vi.mock('@/api/analytics')
vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([]),
}))
mockApiClient()

function renderTab(projectId = 'myproject') {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/analytics', element: <AnalyticsTab /> }],
    { initialEntries: [`/projects/${projectId}/analytics`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('AnalyticsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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

  it('shows the empty state message with a filter row above it', async () => {
    renderTab()
    await waitFor(() => {
      expect(screen.getByText(/no report data yet/i)).toBeInTheDocument()
    })
    // PageHeader (title/heading) is now owned by the project layout, not the tab itself.
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  })
})
