import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { LabelBreakdownCard } from '../LabelBreakdownCard'
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

function renderCard(projectId = 'myproject') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <LabelBreakdownCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('LabelBreakdownCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders with default label "severity"', () => {
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Label Breakdown')).toBeInTheDocument()
    expect(analyticsApi.fetchLabelBreakdown).toHaveBeenCalledWith('myproject', 'severity', 20)
  })

  it('renders pie chart when data is available', async () => {
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockResolvedValue({
      data: [
        { value: 'critical', count: 15 },
        { value: 'normal', count: 30 },
        { value: 'minor', count: 10 },
      ],
      project_id: 'myproject',
    })
    renderCard()
    await waitFor(() => {
      expect(document.querySelector('[data-chart]')).not.toBeNull()
    })
  })

  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockResolvedValue({
      data: [],
      project_id: 'myproject',
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('No label data available')).toBeInTheDocument()
    })
  })

  it('label selector triggers new fetch with selected label', async () => {
    const user = userEvent.setup()
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockResolvedValue({
      data: [{ value: 'critical', count: 5 }],
      project_id: 'myproject',
    })
    renderCard()

    // Wait for initial render
    await waitFor(() => {
      expect(analyticsApi.fetchLabelBreakdown).toHaveBeenCalledWith('myproject', 'severity', 20)
    })

    // Click the select trigger to open dropdown
    const trigger = screen.getByRole('combobox')
    await user.click(trigger)

    // Select "feature" option
    const featureOption = await screen.findByRole('option', { name: 'feature' })
    await user.click(featureOption)

    await waitFor(() => {
      expect(analyticsApi.fetchLabelBreakdown).toHaveBeenCalledWith('myproject', 'feature', 20)
    })
  })
})
