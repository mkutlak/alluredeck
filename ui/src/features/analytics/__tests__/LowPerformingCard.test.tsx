import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { LowPerformingCard } from '../LowPerformingCard'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

describe('LowPerformingCard', () => {
  it('renders slowest tests by default', async () => {
    vi.mocked(reportsApi.fetchLowPerformingTests).mockResolvedValue({
      tests: [
        {
          test_name: 'SlowTest',
          full_name: 'pkg.SlowTest',
          history_id: 'h1',
          metric: 5432,
          build_count: 3,
          trend: [4000, 5000, 5432],
        },
      ],
      sort: 'duration',
      builds: 20,
      total: 1,
    })
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <LowPerformingCard projectId="myproject" />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('pkg.SlowTest')).toBeInTheDocument()
  })
})
