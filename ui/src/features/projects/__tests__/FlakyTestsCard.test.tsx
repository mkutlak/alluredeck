import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { FlakyTestsCard } from '../FlakyTestsCard'
import { ApiError } from '@/api/client'
import * as reportsApi from '@/api/reports'
import type { StabilityData } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function stability(flakyRetries: number[]): StabilityData {
  return {
    flaky_tests: flakyRetries.map((retries_count) => ({
      name: 'TestLogin',
      full_name: 'pkg.TestLogin',
      status: 'failed',
      retries_count,
      retries_status_change: true,
    })),
    new_failed: [],
    new_passed: [],
    summary: {
      flaky_count: flakyRetries.length,
      retried_count: flakyRetries.length,
      new_failed_count: 0,
      new_passed_count: 0,
      total: 10,
    },
  }
}

function renderCard(numericProjectId?: number) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <FlakyTestsCard projectId="myproject" numericProjectId={numericProjectId} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('FlakyTestsCard', () => {
  it('shows a flaky badge with the retry count for each test', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue(stability([3]))
    renderCard()

    expect(await screen.findByText('TestLogin')).toBeInTheDocument()
    expect(screen.getByText('flaky · 3x')).toBeInTheDocument()
  })

  // Links use the numeric project_id, never the slug route param (ui/CLAUDE.md).
  it.each<[string, number | undefined, string | null]>([
    [
      'shows a link to the flaky-impact analytics view when the numeric project id is known',
      42,
      '/projects/42/analytics',
    ],
    [
      'does not show the flaky-impact link when the numeric project id is unresolved',
      undefined,
      null,
    ],
  ])('%s', async (_name, id, href) => {
    vi.mocked(reportsApi.fetchReportStability).mockResolvedValue(stability([1]))
    renderCard(id)
    await screen.findByText('TestLogin')

    expect(screen.queryByTestId('flaky-impact-link')?.getAttribute('href') ?? null).toBe(href)
  })

  // A 404 means no stability data yet — for this auxiliary card that is "nothing to show".
  it.each<[string, () => Promise<StabilityData>]>([
    ['renders nothing when no flaky tests', () => Promise.resolve(stability([]))],
    [
      'renders nothing on 404 — missing stability data is not an error',
      () => Promise.reject(new ApiError('build not found', { status: 404, data: {} })),
    ],
  ])('%s', async (_name, respond) => {
    vi.mocked(reportsApi.fetchReportStability).mockImplementation(respond)
    const { container } = renderCard()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })

  it('keeps the error state for non-404 failures', async () => {
    vi.mocked(reportsApi.fetchReportStability).mockRejectedValue(
      new ApiError('boom', { status: 500, data: {} }),
    )
    renderCard()
    expect(await screen.findByText(/couldn't load data/i)).toBeInTheDocument()
  })
})
