import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { SuitePassRateChart } from '../SuitePassRateChart'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/analytics')
mockApiClient()

describe('SuitePassRateChart', () => {
  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchSuitePassRates).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <SuitePassRateChart projectId="myproject" />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('No suite data available')).toBeInTheDocument()
  })

  // --chart-1 is a bare HSL triplet ("109 58% 40%"), not a colour: the bars need hsl() around it.
  it('colours the bars with the chart-1 theme colour as an hsl() value', async () => {
    vi.mocked(analyticsApi.fetchSuitePassRates).mockResolvedValue({
      data: [{ suite: 'checkout', pass_rate: 80, passed: 8, total: 10 }],
      metadata: { message: 'ok' },
    })
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <SuitePassRateChart projectId="myproject" />
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/--color-pass_rate:/, { ignore: false })).toHaveTextContent(
      '--color-pass_rate: hsl(var(--chart-1));',
    )
  })
})
