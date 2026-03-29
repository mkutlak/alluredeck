import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ProjectDefectsView } from '../ProjectDefectsView'
import * as defectsApi from '@/api/defects'
import type { DefectProjectSummary } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/defects')
mockApiClient()

function makeSummary(overrides: Partial<DefectProjectSummary> = {}): DefectProjectSummary {
  return {
    open: 10,
    fixed: 5,
    muted: 3,
    wont_fix: 1,
    regressions_last_build: 2,
    by_category: { product_bug: 8, test_bug: 6, infrastructure: 3, to_investigate: 2 },
    ...overrides,
  }
}

function renderPage(projectId = 'myproject') {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/defects', element: <ProjectDefectsView /> }],
    { initialEntries: [`/projects/${projectId}/defects`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('ProjectDefectsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders project heading', async () => {
    vi.mocked(defectsApi.fetchProjectDefectSummary).mockResolvedValue(makeSummary())
    vi.mocked(defectsApi.fetchProjectDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText('myproject')).toBeInTheDocument()
    })
    expect(screen.getByText('Defect Grouping')).toBeInTheDocument()
  })

  it('renders summary cards with correct values', async () => {
    vi.mocked(defectsApi.fetchProjectDefectSummary).mockResolvedValue(makeSummary())
    vi.mocked(defectsApi.fetchProjectDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByTestId('summary-open')).toHaveTextContent('10')
    })
    expect(screen.getByTestId('summary-fixed')).toHaveTextContent('5')
    expect(screen.getByTestId('summary-muted')).toHaveTextContent('3')
    expect(screen.getByTestId('summary-regressions')).toHaveTextContent('2')
  })

  it('shows trend placeholder', async () => {
    vi.mocked(defectsApi.fetchProjectDefectSummary).mockResolvedValue(makeSummary())
    vi.mocked(defectsApi.fetchProjectDefects).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 25, total_pages: 0 },
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/defect trends coming soon/i)).toBeInTheDocument()
    })
  })
})
