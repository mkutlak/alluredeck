import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EnvironmentCard } from '../EnvironmentCard'
import * as reportsApi from '@/api/reports'

vi.mock('@/api/reports')
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function renderCard(projectId = 'myproject') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <EnvironmentCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('EnvironmentCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons initially', () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Environment')).toBeInTheDocument()
  })

  it('renders environment entries', async () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([
      { name: 'Browser', values: ['Chrome 120'] },
      { name: 'OS', values: ['Linux', 'macOS'] },
    ])
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('Browser')).toBeInTheDocument()
      expect(screen.getByText('Chrome 120')).toBeInTheDocument()
      expect(screen.getByText('Linux, macOS')).toBeInTheDocument()
    })
  })

  it('renders nothing when no entries', async () => {
    vi.mocked(reportsApi.fetchReportEnvironment).mockResolvedValue([])
    const { container } = renderCard()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })
})
