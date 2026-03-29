import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { SuitePassRateChart } from '../SuitePassRateChart'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/analytics')
mockApiClient()

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
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
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
      metadata: { message: 'ok' },
    })
    renderChart()
    await waitFor(() => {
      expect(document.querySelector('[data-chart]')).not.toBeNull()
    })
  })

  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchSuitePassRates).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
    renderChart()
    await waitFor(() => {
      expect(screen.getByText('No suite data available')).toBeInTheDocument()
    })
  })
})
