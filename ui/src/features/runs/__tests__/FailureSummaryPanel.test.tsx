import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { createTestQueryClient } from '@/test/render'
import { FailureSummaryPanel } from '../FailureSummaryPanel'
import type { FailureSummaryData } from '@/types/api'

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn(),
}))
vi.mock('@/api/system', () => ({
  getConfig: vi.fn(),
}))

import { fetchFailureSummary } from '@/api/failures'

function makeSummaryData(overrides: Partial<FailureSummaryData> = {}): FailureSummaryData {
  return {
    enabled: true,
    cached: false,
    build_id: 123,
    history_id: 'abc123',
    summary: {
      hypothesis: 'The login handler throws because `user` is undefined.',
      category: 'product_bug',
      confidence: 'medium',
      evidence: ['TypeError: Cannot read properties of undefined', 'Occurred at line 42'],
    },
    last_good: { build_number: 41, commit_sha: '9af3xyz', builds_since: 3 },
    model: 'llama3.1',
    generated_at: '2026-07-05T12:00:00Z',
    disclaimer: 'AI hypothesis — verify before acting.',
    ...overrides,
  }
}

// /config is seeded (fresh for its 5-minute staleTime) so the llm_enabled gate is
// decided on the first render and "renders nothing" cannot pass before it loads.
function renderPanel({ llm = true, open = true } = {}) {
  const qc = createTestQueryClient()
  qc.setQueryData(['config'], { data: { llm_enabled: llm }, metadata: { message: 'ok' } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open={open} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('FailureSummaryPanel', () => {
  beforeEach(() => {
    vi.mocked(fetchFailureSummary).mockReset()
  })

  it.each([
    { name: 'llm_enabled is false', llm: false, open: true },
    { name: 'not open', llm: true, open: false },
  ])('renders nothing and does not fetch when $name', ({ llm, open }) => {
    renderPanel({ llm, open })
    expect(screen.queryByTestId('failure-summary-panel')).not.toBeInTheDocument()
    expect(fetchFailureSummary).not.toHaveBeenCalled()
  })

  it('renders hypothesis, category, confidence, evidence, last-good link, and disclaimer on success', async () => {
    vi.mocked(fetchFailureSummary).mockResolvedValue(makeSummaryData())
    renderPanel()

    expect(await screen.findByText('AI hypothesis')).toBeInTheDocument()
    expect(screen.getByText('product_bug')).toBeInTheDocument()
    expect(screen.getByText('medium confidence')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByTestId('markdown-content').innerHTML).toContain('login handler throws')
    })
    expect(screen.getByText('TypeError: Cannot read properties of undefined')).toBeInTheDocument()
    expect(screen.getByText('Occurred at line 42')).toBeInTheDocument()
    expect(screen.getByText('AI hypothesis — verify before acting.')).toBeInTheDocument()

    const link = screen.getByRole('link', { name: /last passed/i })
    expect(link).toHaveAttribute('href', '/projects/1/reports/41')
    expect(link).toHaveTextContent('build #41')
    expect(link).toHaveTextContent('3 builds ago')
  })

  // LLM non-JSON fallback: evidence null, empty category, no confidence, no last_good.
  it('omits the evidence list, empty pills, and last-good link when those fields are absent', async () => {
    vi.mocked(fetchFailureSummary).mockResolvedValue(
      makeSummaryData({
        summary: { hypothesis: 'Fallback hypothesis text.', category: '', evidence: null },
        last_good: undefined,
      }),
    )
    renderPanel()

    const badge = await screen.findByText('AI hypothesis')
    expect(badge.parentElement?.children).toHaveLength(1)
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /last passed/i })).not.toBeInTheDocument()
  })

  it('shows a soft error state when summary is null', async () => {
    vi.mocked(fetchFailureSummary).mockResolvedValue(
      makeSummaryData({ summary: null, error: 'generation failed' }),
    )
    renderPanel()

    expect(await screen.findByTestId('failure-summary-soft-error')).toHaveTextContent(
      'generation failed',
    )
  })

  it('renders nothing when the server reports the feature disabled despite local config', async () => {
    vi.mocked(fetchFailureSummary).mockResolvedValue({ enabled: false })
    renderPanel()

    // The panel shows while the summary loads, then disappears on enabled: false.
    expect(screen.getByTestId('failure-summary-panel')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.queryByTestId('failure-summary-panel')).not.toBeInTheDocument()
    })
  })
})
