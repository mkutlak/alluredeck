import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type { PipelineRun, RunFailure } from '@/types/api'

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

function makeRun(branch: string): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234',
    branch,
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
        failed: 2,
        duration_ms: 30000,
        status: 'degraded',
        builds: [{ build_id: 103, build_number: 3 }],
      },
    ],
    aggregate: {
      suites_passed: 0,
      suites_total: 1,
      tests_passed: 98,
      tests_total: 100,
      pass_rate: 98,
      total_duration_ms: 30000,
    },
  }
}

function failure(history_id: string): RunFailure {
  return {
    project_id: 2,
    slug: 'ui-tests',
    build_id: 103,
    build_number: 3,
    test_name: 'should login',
    full_name: 'login.spec.js:1:1',
    status: 'failed',
    duration_ms: 100,
    history_id,
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'TimeoutError',
  }
}

function renderFeedPanel(run: PipelineRun, history_id = 'h1') {
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: [failure(history_id)],
    metadata: { message: 'ok', truncated: false },
  })
  return renderWithProviders(<RunFailures run={run} />)
}

beforeEach(() => {
  vi.mocked(fetchRunFailures).mockReset()
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

// The run page does this already; the feed's inline panel is the same list.
describe('RunFailures in the feed', () => {
  it.each([
    { name: 'a branch', branch: 'feature/a b', query: 'history_id=h1&branch=feature%2Fa+b' },
    { name: 'no branch', branch: '', query: 'history_id=h1' },
  ])('links a test to its history, with $name', async ({ branch, query }) => {
    renderFeedPanel(makeRun(branch))

    expect(await screen.findByRole('link', { name: 'should login' })).toHaveAttribute(
      'href',
      `/projects/2/tests?${query}`,
    )
    expect(screen.getByRole('link', { name: 'Allure report for should login' })).toHaveAttribute(
      'href',
      '/projects/2/reports/3',
    )
  })

  it('keeps the report link for a test without a history id', async () => {
    renderFeedPanel(makeRun('main'), '')

    expect(await screen.findByRole('link', { name: 'should login' })).toHaveAttribute(
      'href',
      '/projects/2/reports/3',
    )
    expect(screen.queryByRole('link', { name: /allure report for/i })).not.toBeInTheDocument()
  })
})
