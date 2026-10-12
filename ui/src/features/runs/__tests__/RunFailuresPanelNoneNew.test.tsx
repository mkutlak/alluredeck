import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type { PipelineRun, PipelineSuite, RunFailure } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi
    .fn()
    .mockResolvedValue({ data: { llm_enabled: false }, metadata: { message: 'ok' } }),
}))

import { fetchRunFailures } from '@/api/pipeline'

const listedSuite: PipelineSuite = {
  project_id: 2,
  slug: 'ui-tests',
  build_number: 3,
  build_id: 103,
  pass_rate: 98,
  total: 100,
  failed: 2,
  duration_ms: 30000,
  status: 'degraded',
  builds: [{ build_id: 103, build_number: 3 }],
}

// Failed, but its report could not be read: no counts, so no test of it was ever listed.
const unreadableSuite: PipelineSuite = {
  ...listedSuite,
  project_id: 4,
  slug: 'api-tests',
  build_number: 7,
  build_id: 107,
  pass_rate: 0,
  total: 0,
  failed: 0,
  status: 'failed',
  builds: [{ build_id: 107, build_number: 7 }],
}

function makeRun(suites: PipelineSuite[], awaiting_suites?: number): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites,
    aggregate: {
      suites_passed: 0,
      suites_total: suites.length,
      tests_passed: 98,
      tests_total: 100,
      pass_rate: 98,
      total_duration_ms: 30000,
      awaiting_suites,
    },
  }
}

function inherited(test_name: string): RunFailure {
  return {
    project_id: 2,
    slug: 'ui-tests',
    build_id: 103,
    build_number: 3,
    test_name,
    full_name: test_name,
    status: 'failed',
    duration_ms: 100,
    history_id: `h-${test_name}`,
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'TimeoutError: locator.click',
    origin: 'inherited',
  }
}

function renderPanel(run: PipelineRun) {
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: [inherited('i1'), inherited('i2')],
    metadata: { message: 'ok', truncated: false },
  })
  return renderWithProviders(<RunFailures run={run} />)
}

beforeEach(() => {
  vi.mocked(fetchRunFailures).mockReset()
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

// The row and the run page withhold "none new" when the run may hold failures that were never
// listed (canSayNoneNew); the panel header must not say it either.
describe('failures panel header: "none new"', () => {
  it('leads with "none new" when the run is finished and every listed failure is inherited', async () => {
    renderPanel(makeRun([listedSuite]))

    expect(await screen.findByTestId('run-failure-origins')).toHaveTextContent(
      /^none new · 2 inherited$/,
    )
  })

  it('does not say "none new" while suites are still awaited', async () => {
    renderPanel(makeRun([listedSuite], 2))

    expect(await screen.findByTestId('run-failure-origins')).toHaveTextContent(/^2 inherited$/)
  })

  it('does not say "none new" beside a failed suite whose report could not be read', async () => {
    renderPanel(makeRun([listedSuite, unreadableSuite]))

    expect(await screen.findByTestId('run-failure-origins')).toHaveTextContent(/^2 inherited$/)
  })
})
