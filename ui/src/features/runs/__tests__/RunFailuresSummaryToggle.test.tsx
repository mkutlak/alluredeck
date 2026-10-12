import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type { ApiResponse, ConfigData, PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn().mockResolvedValue({
    data: [
      {
        project_id: 2,
        slug: 'ui-tests',
        build_id: 103,
        build_number: 3,
        test_name: 'should login',
        full_name: 'login.spec.js:1:1',
        status: 'failed',
        duration_ms: 100,
        history_id: 'h1',
        flaky: false,
        retries: 0,
        new_failed: false,
        known: false,
        error_message: 'TimeoutError: locator.click',
      },
    ],
    metadata: { message: 'ok', truncated: false },
  }),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn().mockResolvedValue({
    enabled: true,
    summary: { hypothesis: 'Looks like a stale selector.', evidence: [] },
    disclaimer: 'AI hypothesis — verify before acting.',
  }),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi.fn().mockResolvedValue({
    data: { llm_enabled: true } as ConfigData,
    metadata: { message: 'ok' },
  } satisfies ApiResponse<ConfigData>),
}))

const run: PipelineRun = {
  pipeline_id: '196765',
  commit_sha: 'abc1234',
  branch: 'main',
  timestamp: '2026-04-03T18:00:00Z',
  group_project_id: 10,
  group_slug: 'acme',
  suites: [
    {
      project_id: 2,
      slug: 'ui-tests',
      build_number: 3,
      build_id: 103,
      pass_rate: 98,
      total: 100,
      failed: 1,
      duration_ms: 30000,
      status: 'degraded',
      builds: [{ build_id: 103, build_number: 3 }],
    },
  ],
  aggregate: {
    suites_passed: 0,
    suites_total: 1,
    tests_passed: 99,
    tests_total: 100,
    pass_rate: 99,
    total_duration_ms: 30000,
  },
}

beforeEach(() => {
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

describe('the AI failure summary toggle', () => {
  // A bare sparkle said nothing to anyone who had not already found it.
  it('says what it opens, and points at the summary while it is open', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RunFailures run={run} />)
    const toggle = await screen.findByRole('button', {
      name: 'Toggle AI failure summary for should login',
    })
    expect(toggle).toHaveTextContent('AI summary')
    expect(toggle).not.toHaveAttribute('aria-controls')

    await user.click(toggle)
    const panel = document.getElementById(toggle.getAttribute('aria-controls') ?? '')
    expect(panel).toContainElement(await screen.findByTestId('failure-summary-panel'))
  })
})
