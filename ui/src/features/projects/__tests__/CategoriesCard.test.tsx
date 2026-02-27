import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { CategoriesCard } from '../CategoriesCard'
import * as reportsApi from '@/api/reports'

vi.mock('@/api/reports')
vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function renderCard(projectId = 'myproject') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
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

  it('shows empty state when no categories', async () => {
    vi.mocked(reportsApi.fetchReportCategories).mockResolvedValue([])
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('No defect categories')).toBeInTheDocument()
    })
  })
})
