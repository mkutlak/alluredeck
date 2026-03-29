import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { BuildDefectsView } from '../BuildDefectsView'
import * as defectsApi from '@/api/defects'
import type { DefectBuildSummary } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/defects')
mockApiClient()

function makeBuildSummary(overrides: Partial<DefectBuildSummary> = {}): DefectBuildSummary {
  return {
    total_groups: 8,
    affected_tests: 15,
    new_defects: 3,
    regressions: 2,
    by_category: { product_bug: 5, test_bug: 3 },
    by_resolution: { open: 6, muted: 2 },
    ...overrides,
  }
}

function renderPage(projectId = 'myproject', buildId = '42') {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/builds/:buildId/defects', element: <BuildDefectsView /> }],
    { initialEntries: [`/projects/${projectId}/builds/${buildId}/defects`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('BuildDefectsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders build heading', async () => {
    vi.mocked(defectsApi.fetchBuildDefectSummary).mockResolvedValue(makeBuildSummary())
    vi.mocked(defectsApi.fetchBuildDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('myproject')).toBeInTheDocument()
    })
    expect(screen.getByText(/Build #42/)).toBeInTheDocument()
  })

  it('renders summary badges', async () => {
    vi.mocked(defectsApi.fetchBuildDefectSummary).mockResolvedValue(makeBuildSummary())
    vi.mocked(defectsApi.fetchBuildDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/Groups: 8/)).toBeInTheDocument()
    })
    expect(screen.getByText(/Affected tests: 15/)).toBeInTheDocument()
    expect(screen.getByText(/New: 3/)).toBeInTheDocument()
    expect(screen.getByText(/Regressions: 2/)).toBeInTheDocument()
  })

  it('shows back link to project defects', async () => {
    vi.mocked(defectsApi.fetchBuildDefectSummary).mockResolvedValue(makeBuildSummary())
    vi.mocked(defectsApi.fetchBuildDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/Back to project defects/)).toBeInTheDocument()
    })
  })

  it('shows error for invalid build ID', () => {
    const router = createMemoryRouter(
      [{ path: '/projects/:id/builds/:buildId/defects', element: <BuildDefectsView /> }],
      { initialEntries: ['/projects/myproject/builds/abc/defects'] },
    )
    renderWithProviders(<></>, { router })
    expect(screen.getByText(/Invalid project or build ID/)).toBeInTheDocument()
  })
})
