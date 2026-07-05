import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { FailureSummaryPanel } from '../FailureSummaryPanel'
import type { ApiResponse, ConfigData, FailureSummaryData } from '@/types/api'

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn(),
}))
vi.mock('@/api/system', () => ({
  getConfig: vi.fn(),
}))

import { fetchFailureSummary } from '@/api/failures'
import { getConfig } from '@/api/system'

function makeConfig(overrides: Partial<ConfigData> = {}): ApiResponse<ConfigData> {
  return {
    data: {
      version: '1.0.0',
      dev_mode: false,
      check_results_every_seconds: '60',
      keep_history: true,
      keep_history_latest: 10,
      tls: false,
      security_enabled: true,
      url_prefix: '',
      api_response_less_verbose: false,
      optimize_storage: false,
      make_viewer_endpoints_public: false,
      oidc_enabled: false,
      llm_enabled: true,
      ...overrides,
    },
    metadata: { message: 'ok' },
  }
}

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

describe('FailureSummaryPanel', () => {
  beforeEach(() => {
    vi.mocked(fetchFailureSummary).mockReset()
    vi.mocked(getConfig).mockReset()
  })

  it('renders nothing when llm_enabled is false', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig({ llm_enabled: false }))
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(getConfig).toHaveBeenCalled()
    })
    expect(screen.queryByTestId('failure-summary-panel')).not.toBeInTheDocument()
    expect(fetchFailureSummary).not.toHaveBeenCalled()
  })

  it('renders nothing when not open, and does not fetch', () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    renderWithProviders(
      <FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open={false} />,
    )
    expect(screen.queryByTestId('failure-summary-panel')).not.toBeInTheDocument()
    expect(fetchFailureSummary).not.toHaveBeenCalled()
  })

  it('shows a loading state before data arrives', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockReturnValue(new Promise(() => {}))
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(screen.getByTestId('failure-summary-panel')).toBeInTheDocument()
    })
    expect(screen.queryByText('AI hypothesis')).not.toBeInTheDocument()
  })

  it('renders hypothesis, category, confidence, evidence, last-good link, and disclaimer on success', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue(makeSummaryData())
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(screen.getByText('AI hypothesis')).toBeInTheDocument()
    })
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

  it('renders without a last-good link when last_good is absent, and does not crash', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue(makeSummaryData({ last_good: undefined }))
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(screen.getByText('AI hypothesis')).toBeInTheDocument()
    })
    expect(screen.queryByRole('link', { name: /last passed/i })).not.toBeInTheDocument()
  })

  it('renders evidence-free when evidence is null (LLM non-JSON fallback), without crashing', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue(
      makeSummaryData({
        summary: {
          hypothesis: 'Fallback hypothesis text.',
          category: 'test_bug',
          evidence: null,
        },
      }),
    )
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(screen.getByText('AI hypothesis')).toBeInTheDocument()
    })
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('renders evidence-free when the evidence key is absent, without crashing', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue(
      makeSummaryData({
        summary: {
          hypothesis: 'Fallback hypothesis text.',
          category: 'test_bug',
        },
      }),
    )
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(screen.getByText('AI hypothesis')).toBeInTheDocument()
    })
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('does not render an empty category pill when category is an empty string', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue(
      makeSummaryData({
        summary: { hypothesis: 'Unclear hypothesis.', category: '', evidence: [] },
      }),
    )
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    const badge = await screen.findByText('AI hypothesis')
    // Only the "AI hypothesis" badge should render in the badge row — no
    // empty category pill (and no confidence pill, since it's also absent).
    expect(badge.parentElement?.children).toHaveLength(1)
  })

  it('shows a soft error state when summary is null', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue(
      makeSummaryData({ summary: null, error: 'generation failed' }),
    )
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(screen.getByTestId('failure-summary-soft-error')).toBeInTheDocument()
    })
    expect(screen.getByText('generation failed')).toBeInTheDocument()
  })

  it('renders nothing when the server reports the feature disabled despite local config', async () => {
    vi.mocked(getConfig).mockResolvedValue(makeConfig())
    vi.mocked(fetchFailureSummary).mockResolvedValue({ enabled: false })
    renderWithProviders(<FailureSummaryPanel projectId={1} buildId={123} historyId="abc123" open />)

    await waitFor(() => {
      expect(fetchFailureSummary).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(screen.queryByTestId('failure-summary-panel')).not.toBeInTheDocument()
    })
  })
})
