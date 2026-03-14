import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { CategoriesCard } from '../CategoriesCard'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderCard(projectId = 'myproject') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <CategoriesCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('CategoriesCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading state initially', () => {
    vi.mocked(reportsApi.fetchReportCategories).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Failure Categories')).toBeInTheDocument()
  })

  it('renders categories with badges', async () => {
    vi.mocked(reportsApi.fetchReportCategories).mockResolvedValue([
      {
        name: 'Product defects',
        matchedStatistic: { failed: 3, broken: 0, known: 0, unknown: 0, total: 3 },
      },
      {
        name: 'Test defects',
        matchedStatistic: { failed: 0, broken: 2, known: 0, unknown: 0, total: 2 },
      },
    ])
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('Product defects')).toBeInTheDocument()
      expect(screen.getByText('Test defects')).toBeInTheDocument()
      expect(screen.getByText('3f')).toBeInTheDocument()
      expect(screen.getByText('2b')).toBeInTheDocument()
    })
  })

  it('renders nothing when no categories', async () => {
    vi.mocked(reportsApi.fetchReportCategories).mockResolvedValue([])
    const { container } = renderCard()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })
})
