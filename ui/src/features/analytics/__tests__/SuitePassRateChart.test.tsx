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
})
