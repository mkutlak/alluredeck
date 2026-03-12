import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SuitePassRateChart } from '../SuitePassRateChart'
import * as analyticsApi from '@/api/analytics'

vi.mock('@/api/analytics')
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

// Recharts uses ResizeObserver + SVG layout; stub it out in jsdom
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div style={{ width: 500, height: 300 }}>{children}</div>
    ),
  }
})

function renderChart(projectId = 'myproject') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <SuitePassRateChart projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('SuitePassRateChart', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows title while loading', () => {
    vi.mocked(analyticsApi.fetchSuitePassRates).mockReturnValue(new Promise(() => {}))
    renderChart()
    expect(screen.getByText('Suite Pass Rates')).toBeInTheDocument()
  })

  it('renders chart container when data is available', async () => {
    vi.mocked(analyticsApi.fetchSuitePassRates).mockResolvedValue({
      data: [
        { suite: 'Auth Suite', total: 50, passed: 45, pass_rate: 90 },
        { suite: 'Payment Suite', total: 30, passed: 28, pass_rate: 93.3 },
      ],
      project_id: 'myproject',
    })
    renderChart()
    await waitFor(() => {
      expect(document.querySelector('[data-chart]')).not.toBeNull()
    })
  })

  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchSuitePassRates).mockResolvedValue({
      data: [],
      project_id: 'myproject',
    })
    renderChart()
    await waitFor(() => {
      expect(screen.getByText('No suite data available')).toBeInTheDocument()
    })
  })
})
