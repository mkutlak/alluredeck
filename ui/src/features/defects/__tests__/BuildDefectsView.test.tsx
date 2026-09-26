import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { BuildDefectsView } from '../BuildDefectsView'
import * as defectsApi from '@/api/defects'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/defects')
mockApiClient()

function renderPage(buildId: string) {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/builds/:buildId/defects', element: <BuildDefectsView /> }],
    { initialEntries: [`/projects/myproject/builds/${buildId}/defects`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('BuildDefectsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the build heading and its summary badges', async () => {
    vi.mocked(defectsApi.fetchBuildDefectSummary).mockResolvedValue({
      total_groups: 8,
      affected_tests: 15,
      new_defects: 3,
      regressions: 2,
      by_category: { product_bug: 5, test_bug: 3 },
      by_resolution: { open: 6, muted: 2 },
    })
    vi.mocked(defectsApi.fetchBuildDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage('42')

    expect(await screen.findByText(/Groups: 8/)).toBeInTheDocument()
    expect(defectsApi.fetchBuildDefectSummary).toHaveBeenCalledWith('myproject', 42)
    expect(screen.getByText(/Build #42/)).toBeInTheDocument()
    expect(screen.getByText(/Affected tests: 15/)).toBeInTheDocument()
    expect(screen.getByText(/New: 3/)).toBeInTheDocument()
    expect(screen.getByText(/Regressions: 2/)).toBeInTheDocument()
  })

  it('shows error for invalid build ID', () => {
    renderPage('abc')
    expect(screen.getByText(/Invalid project or build ID/)).toBeInTheDocument()
  })
})
