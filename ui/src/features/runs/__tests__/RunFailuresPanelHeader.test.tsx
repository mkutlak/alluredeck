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
  getConfig: vi
    .fn()
    .mockResolvedValue({ data: { llm_enabled: false }, metadata: { message: 'ok' } }),
}))

import { fetchRunFailures } from '@/api/pipeline'

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
      failed: 6,
      duration_ms: 30000,
      status: 'degraded',
      builds: [{ build_id: 103, build_number: 3 }],
    },
  ],
  aggregate: {
    suites_passed: 0,
    suites_total: 1,
    tests_passed: 94,
    tests_total: 100,
    pass_rate: 94,
    total_duration_ms: 30000,
  },
}

function failure(test_name: string, extra?: Partial<RunFailure>): RunFailure {
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
    ...extra,
  }
}

// 2 new, 3 inherited (one of them an environment failure), 1 unknown.
const tagged = [
  failure('n1', { origin: 'new' }),
  failure('n2', { origin: 'new' }),
  failure('i1', { origin: 'inherited' }),
  failure('i2', { origin: 'inherited' }),
  failure('e1', { origin: 'new', environment: true }),
  failure('u1'),
]

function renderPanel(failures: RunFailure[], { asPage = false, truncated = false } = {}) {
  const response: RunFailuresResponse = { data: failures, metadata: { message: 'ok', truncated } }
  vi.mocked(fetchRunFailures).mockResolvedValue(response)
  return renderWithProviders(<RunFailures run={run} asPage={asPage} />)
}

beforeEach(() => {
  vi.mocked(fetchRunFailures).mockReset()
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

// The collapsed row and its chips already say how many tests failed in how many
// suites; the panel header keeps only what they do not: where the failures came from.
describe.each([
  { mode: 'feed', asPage: false },
  { mode: 'run page', asPage: true },
])('failures panel header ($mode)', ({ asPage }) => {
  it('shows the origin counts and no failure or suite count', async () => {
    renderPanel(tagged, { asPage })

    expect(await screen.findByText('2 new · 3 inherited (1 env) · 1 unknown')).toBeInTheDocument()
    expect(screen.queryByText(/\bfailures? ·/)).not.toBeInTheDocument()
    expect(screen.queryByText(/· \d+ suites?\b/)).not.toBeInTheDocument()
  })

  it('counts the failures in the header as unknown when none has a known origin', async () => {
    renderPanel([failure('u1'), failure('u2'), failure('u3', { origin: 'unknown' })], { asPage })

    await screen.findAllByTestId('run-failure-row')
    expect(screen.queryByText(/^\d+ failures?\b/)).not.toBeInTheDocument()
    expect(screen.queryByText(/\d+ suites?\b/)).not.toBeInTheDocument()
    expect(screen.getByTestId('run-failure-origins')).toHaveTextContent(/^3 unknown$/)
  })

  it('says the counts cover only the listed failures when the response was truncated', async () => {
    renderPanel(tagged.slice(0, 3), { asPage, truncated: true })

    expect(await screen.findByText('first 3: 2 new · 1 inherited')).toBeInTheDocument()
    expect(screen.queryByText(/\bfailures? ·/)).not.toBeInTheDocument()
    expect(screen.getByText(/Showing the first 3 failures; this run has more/)).toBeInTheDocument()
  })
})
