import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { FlakyTestsCard } from '../FlakyTestsCard'
import { ApiError } from '@/api/client'
import * as reportsApi from '@/api/reports'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderCard(projectId = 'myproject', numericProjectId?: number) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <FlakyTestsCard projectId={projectId} numericProjectId={numericProjectId} />
      </MemoryRouter>
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

  it('shows a flaky badge with the retry count for each test', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue({
      flaky_tests: [
        {
          name: 'TestLogin',
          full_name: 'pkg.TestLogin',
          status: 'failed',
          retries_count: 3,
          retries_status_change: true,
        },
      ],
      new_failed: [],
      new_passed: [],
      summary: {
        flaky_count: 1,
        retried_count: 3,
        new_failed_count: 0,
        new_passed_count: 0,
        total: 10,
      },
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('flaky · 3x')).toBeInTheDocument()
    })
  })

  it('shows a link to the flaky-impact analytics view when the numeric project id is known', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue({
      flaky_tests: [
        {
          name: 'TestLogin',
          full_name: 'pkg.TestLogin',
          status: 'failed',
          retries_count: 1,
          retries_status_change: true,
        },
      ],
      new_failed: [],
      new_passed: [],
      summary: {
        flaky_count: 1,
        retried_count: 1,
        new_failed_count: 0,
        new_passed_count: 0,
        total: 10,
      },
    })
    renderCard('myproject', 42)
    await waitFor(() => {
      expect(screen.getByTestId('flaky-impact-link')).toHaveAttribute(
        'href',
        '/projects/42/analytics',
      )
    })
  })

  it('does not show the flaky-impact link when the numeric project id is unresolved', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue({
      flaky_tests: [
        {
          name: 'TestLogin',
          full_name: 'pkg.TestLogin',
          status: 'failed',
          retries_count: 1,
          retries_status_change: true,
        },
      ],
      new_failed: [],
      new_passed: [],
      summary: {
        flaky_count: 1,
        retried_count: 1,
        new_failed_count: 0,
        new_passed_count: 0,
        total: 10,
      },
    })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText('TestLogin')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('flaky-impact-link')).not.toBeInTheDocument()
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

  it('renders nothing on 404 — missing stability data is not an error', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockRejectedValue(
      new ApiError('build not found', { status: 404, data: {} }),
    )
    const { container } = renderCard()
    await waitFor(() => {
      expect(vi.mocked(reportsApi.fetchReportStability)).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })

  it('keeps the error state for non-404 failures', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockRejectedValue(
      new ApiError('boom', { status: 500, data: {} }),
    )
    renderCard()
    await waitFor(() => {
      expect(screen.getByText(/couldn't load data/i)).toBeInTheDocument()
    })
  })
})
