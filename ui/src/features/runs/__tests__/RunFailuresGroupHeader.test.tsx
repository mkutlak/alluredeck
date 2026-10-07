import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type { PipelineRun, RunFailure, RunFailuresResponse } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi.fn(),
}))

import { fetchRunFailures } from '@/api/pipeline'
import { getConfig } from '@/api/system'

const MESSAGE = 'TimeoutError: locator.click'
const STEP = "locator.click: getByRole('button', { name: 'Submit order' })"

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
      pass_rate: 85,
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

const failure: RunFailure = {
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
  error_message: MESSAGE,
}

function respondWith(failures: RunFailure[]) {
  const response: RunFailuresResponse = {
    data: failures,
    metadata: { message: 'ok', truncated: false },
  }
  vi.mocked(fetchRunFailures).mockResolvedValue(response)
}

beforeEach(() => {
  vi.mocked(fetchRunFailures).mockReset()
  vi.mocked(getConfig).mockReset()
  vi.mocked(getConfig).mockResolvedValue({
    data: { llm_enabled: false },
    metadata: { message: 'ok' },
  } as Awaited<ReturnType<typeof getConfig>>)
  useUIStore.setState({ runsFailureGrouping: 'error' })
})

describe('RunFailures by-error group header failed step', () => {
  // Two clusters can share a message and differ only here, so it must be readable in full.
  it('shows the step in the header, as part of the toggle name, beside an unchanged subtitle', async () => {
    respondWith([{ ...failure, failed_step: STEP }])
    renderWithProviders(<RunFailures run={run} />)

    const toggle = await screen.findByRole('button', { name: /TimeoutError: locator\.click/ })
    expect(toggle).toHaveAccessibleName(expect.stringContaining(`in ${STEP}`))
    expect(screen.queryByText(`in ${STEP}`)).toBeInTheDocument()
    expect(screen.getByText('×1 · ui-tests')).toBeInTheDocument()
  })

  it('renders no step line for a group without a step', async () => {
    respondWith([failure])
    renderWithProviders(<RunFailures run={run} />)

    const toggle = await screen.findByRole('button', { name: /TimeoutError: locator\.click/ })
    expect(toggle).toHaveAccessibleName('TimeoutError: locator.click ×1 · ui-tests')
    expect(screen.queryByText(/^in /)).not.toBeInTheDocument()
  })
})
