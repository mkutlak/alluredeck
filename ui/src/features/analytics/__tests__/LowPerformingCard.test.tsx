import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { LowPerformingCard } from '../LowPerformingCard'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderCard(projectId = 'myproject') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <LowPerformingCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('LowPerformingCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows title while loading', () => {
    vi.mocked(reportsApi.fetchLowPerformingTests).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Low Performing Tests')).toBeInTheDocument()
  })

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
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('SlowTest')).toBeInTheDocument()
    })
  })

  it('shows empty state when no data', async () => {
    vi.mocked(reportsApi.fetchLowPerformingTests).mockResolvedValue({
      tests: [],
      sort: 'duration',
      builds: 20,
      total: 0,
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText(/No data yet/)).toBeInTheDocument()
    })
  })
})
