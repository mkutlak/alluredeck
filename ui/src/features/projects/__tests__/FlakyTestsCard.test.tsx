import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { FlakyTestsCard } from '../FlakyTestsCard'
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
      <FlakyTestsCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('FlakyTestsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows title while loading', () => {
    vi.mocked(reportsApi.fetchReportStability).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Flaky Tests')).toBeInTheDocument()
  })

  it('renders flaky test list', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue({
      flaky_tests: [
        {
          name: 'TestLogin',
          full_name: 'pkg.TestLogin',
          status: 'failed',
          retries_count: 2,
          retries_status_change: true,
        },
      ],
      new_failed: [],
      new_passed: [],
      summary: {
        flaky_count: 1,
        retried_count: 2,
        new_failed_count: 0,
        new_passed_count: 0,
        total: 10,
      },
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('TestLogin')).toBeInTheDocument()
    })
  })

  it('shows empty state when no flaky tests', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue({
      flaky_tests: [],
      new_failed: [],
      new_passed: [],
      summary: {
        flaky_count: 0,
        retried_count: 0,
        new_failed_count: 0,
        new_passed_count: 0,
        total: 5,
      },
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('No flaky tests detected')).toBeInTheDocument()
    })
  })
})
