import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { mockRecharts } from '@/test/mocks/recharts'
import { LabelBreakdownCard } from '../LabelBreakdownCard'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/analytics')
mockApiClient()
mockRecharts()

function renderCard() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <LabelBreakdownCard projectId="myproject" />
    </QueryClientProvider>,
  )
}

describe('LabelBreakdownCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches "severity" first and refetches for the selected label', async () => {
    const user = userEvent.setup()
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockResolvedValue({
      data: [{ value: 'critical', count: 5 }],
      metadata: { message: 'ok' },
    })
    renderCard()
    await waitFor(() => {
      expect(analyticsApi.fetchLabelBreakdown).toHaveBeenCalledWith('myproject', 'severity', 20)
    })

    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: 'feature' }))

    await waitFor(() => {
      expect(analyticsApi.fetchLabelBreakdown).toHaveBeenCalledWith('myproject', 'feature', 20)
    })
  })

  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchLabelBreakdown).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
    renderCard()
    expect(await screen.findByText('No label data available')).toBeInTheDocument()
  })
})
