import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
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

async function renderRow(test: FlakyImpact, numericProjectId?: number) {
  vi.mocked(analyticsApi.fetchFlakyImpact).mockResolvedValue({
    tests: [test],
    builds: 20,
    total: 1,
  })
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter>
        <FlakyImpactCard projectId="myproject" numericProjectId={numericProjectId} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return within((await screen.findByText(test.full_name)).closest('tr')!)
}

describe('FlakyImpactCard', () => {
  it('shows flake rate, CI time wasted, retries and builds affected', async () => {
    const row = await renderRow(makeFlakyImpact())

    // builds_affected/runs = 6/20 = 30.0%; 83000ms = 1m 23s
    for (const text of ['30.0%', '1m 23s', '8', '6/20']) {
      expect(row.getByText(text)).toBeInTheDocument()
    }
  })

  it('treats zero runs as a 0% flake rate instead of dividing by zero', async () => {
    const row = await renderRow(makeFlakyImpact({ builds_affected: 0, runs: 0 }))
    expect(row.getByText('0.0%')).toBeInTheDocument()
  })

  // Links use the numeric project_id, never the slug route param (ui/CLAUDE.md).
  it.each<[string, number | undefined, string | null]>([
    [
      'links the last-seen build to its report using the numeric project id',
      42,
      '/projects/42/reports/12',
    ],
    [
      'renders a plain label instead of a link when the numeric project id is unresolved',
      undefined,
      null,
    ],
  ])('%s', async (_name, id, href) => {
    const row = await renderRow(makeFlakyImpact({ last_seen_build_order: 12 }), id)

    expect(row.getByText('#12').closest('a')?.getAttribute('href') ?? null).toBe(href)
  })
})
