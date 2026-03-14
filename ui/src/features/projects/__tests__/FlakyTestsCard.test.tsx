import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { FlakyTestsCard } from '../FlakyTestsCard'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderCard(projectId = 'myproject') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
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

  it('renders nothing when no flaky tests', async () => {
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
    const { container } = renderCard()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })
})
