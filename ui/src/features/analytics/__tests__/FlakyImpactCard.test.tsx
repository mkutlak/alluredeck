import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { FlakyImpactCard } from '../FlakyImpactCard'
import type { FlakyImpact } from '@/types/api'
import * as analyticsApi from '@/api/analytics'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/analytics')
mockApiClient()

function makeFlakyImpact(overrides: Partial<FlakyImpact> = {}): FlakyImpact {
  return {
    full_name: 'suite.should login',
    flaky_count: 5,
    retry_sum: 8,
    wasted_ms: 83_000,
    failure_rate: 0.2,
    runs: 20,
    builds_affected: 6,
    first_seen_build_order: 1,
    first_seen_build_id: 101,
    last_seen_build_order: 12,
    last_seen_build_id: 112,
    last_seen_at: '2026-07-01T10:00:00Z',
    ...overrides,
  }
}

function renderCard(projectId = 'myproject', numericProjectId?: number) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <FlakyImpactCard projectId={projectId} numericProjectId={numericProjectId} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('FlakyImpactCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows title while loading', () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockReturnValue(new Promise(() => {}))
    renderCard()
    expect(screen.getByText('Flaky Impact')).toBeInTheDocument()
  })

  it('shows placeholder when data is empty', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({ tests: [], builds: 20, total: 0 })
    renderCard()
    await waitFor(() => {
      expect(screen.getByText(/no flaky tests detected/i)).toBeInTheDocument()
    })
  })

  it('shows an error state and retries on demand', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockRejectedValue(new Error('boom'))
    renderCard()
    await waitFor(() => {
      expect(screen.getByText(/couldn't load data/i)).toBeInTheDocument()
    })
  })

  it('renders a row with test name, flake rate, retries, and builds affected', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      tests: [makeFlakyImpact()],
      builds: 20,
      total: 1,
    })
    renderCard()

    await waitFor(() => {
      expect(screen.getByText('suite.should login')).toBeInTheDocument()
    })
    // builds_affected/runs = 6/20 = 30.0%
    expect(screen.getByText('30.0%')).toBeInTheDocument()
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('6/20')).toBeInTheDocument()
  })

  it('formats CI time wasted as a human duration', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      tests: [makeFlakyImpact({ wasted_ms: 83_000 })],
      builds: 20,
      total: 1,
    })
    renderCard()

    await waitFor(() => {
      // 83000ms -> 1m 23s
      expect(screen.getByText('1m 23s')).toBeInTheDocument()
    })
  })

  it('links the last-seen build to its report using the numeric project id', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      tests: [makeFlakyImpact({ last_seen_build_order: 12 })],
      builds: 20,
      total: 1,
    })
    renderCard('myproject', 42)

    await waitFor(() => {
      expect(screen.getByRole('link', { name: '#12' })).toHaveAttribute(
        'href',
        '/projects/42/reports/12',
      )
    })
  })

  it('renders a plain label instead of a link when the numeric project id is unresolved', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      tests: [makeFlakyImpact({ last_seen_build_order: 12 })],
      builds: 20,
      total: 1,
    })
    renderCard()

    await waitFor(() => {
      expect(screen.getByText('#12')).toBeInTheDocument()
    })
    expect(screen.queryByRole('link', { name: '#12' })).not.toBeInTheDocument()
  })

  it('colors a mid-range flake rate (10–29%) with the "broken" threshold', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      // builds_affected/runs = 2/20 = 10.0%
      tests: [makeFlakyImpact({ builds_affected: 2, runs: 20 })],
      builds: 20,
      total: 1,
    })
    renderCard()

    await waitFor(() => {
      expect(screen.getByText('10.0%')).toBeInTheDocument()
    })
  })

  it('colors a low flake rate (<10%) with the "passed" threshold', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      // builds_affected/runs = 1/20 = 5.0%
      tests: [makeFlakyImpact({ builds_affected: 1, runs: 20 })],
      builds: 20,
      total: 1,
    })
    renderCard()

    await waitFor(() => {
      expect(screen.getByText('5.0%')).toBeInTheDocument()
    })
  })

  it('treats zero runs as a 0% flake rate instead of dividing by zero', async () => {
    vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
      tests: [makeFlakyImpact({ builds_affected: 0, runs: 0 })],
      builds: 20,
      total: 1,
    })
    renderCard()

    await waitFor(() => {
      expect(screen.getByText('0.0%')).toBeInTheDocument()
    })
  })
})
