import { screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { useUIStore } from '@/store/ui'
import { RunPage } from '../RunPage'
import type { PipelineRun, PipelineSuite, RunFailure } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRun: vi.fn(),
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', display_name: 'Acme Payments', children: [1, 2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi
    .fn()
    .mockResolvedValue({ data: { llm_enabled: false }, metadata: { message: 'ok' } }),
}))

import { fetchPipelineRun, fetchRunFailures } from '@/api/pipeline'

function suite(project_id: number, failed: number, extra?: Partial<PipelineSuite>): PipelineSuite {
  return {
    project_id,
    slug: `suite-${project_id}`,
    build_number: 10 + project_id,
    build_id: 100 + project_id,
    pass_rate: 0,
    total: 100,
    failed,
    duration_ms: 42000,
    status: failed > 0 ? 'degraded' : 'passed',
    ...extra,
  }
}

function makeRun(suites: PipelineSuite[], awaiting_suites?: number): PipelineRun {
  return {
    pipeline_id: '48213',
    commit_sha: 'a1b2c3d4e5f6',
    branch: 'feature/sepa',
    timestamp: '2026-10-07T09:12:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites,
    aggregate: {
      suites_passed: suites.filter((s) => s.status === 'passed').length,
      suites_total: suites.length,
      tests_passed: 197,
      tests_total: 200,
      pass_rate: 98.5,
      total_duration_ms: 213000,
      awaiting_suites,
    },
  }
}

// Every failure is inherited, so the header would lead with "none new".
const inherited: RunFailure[] = ['a1', 'a2', 'a3'].map((test_name) => ({
  project_id: 1,
  slug: 'suite-1',
  build_id: 101,
  build_number: 11,
  test_name,
  full_name: test_name,
  status: 'failed',
  duration_ms: 100,
  history_id: `h-${test_name}`,
  flaky: false,
  retries: 0,
  new_failed: false,
  known: false,
  error_message: 'AssertionError: boom',
  origin: 'inherited',
}))

function renderRun(run: PipelineRun) {
  vi.mocked(fetchPipelineRun).mockResolvedValue({ data: run, metadata: { message: 'ok' } })
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: inherited,
    metadata: { message: 'ok', truncated: false },
  })
  const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
    initialEntries: ['/runs/10/48213'],
  })
  return renderWithProviders(<></>, { router })
}

const summary = () => screen.getByTestId('run-page-summary')

beforeEach(() => {
  vi.mocked(fetchPipelineRun).mockReset()
  vi.mocked(fetchRunFailures).mockReset()
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

describe('RunPage header "none new"', () => {
  it('leads the origins with "none new" when every failure is inherited', async () => {
    renderRun(makeRun([suite(1, 3), suite(2, 0)]))
    expect(await screen.findByText(/none new/)).toBeInTheDocument()
    expect(summary()).toHaveTextContent('none new · 3 inherited')
  })

  it.each([
    { name: 'a suite is still uploading', run: makeRun([suite(1, 3), suite(2, 0)], 2) },
    {
      name: 'a suite failed without a failing test (its report was unreadable)',
      run: makeRun([suite(1, 3), suite(2, 0, { status: 'failed', total: 0 })]),
    },
  ])('does not say it while $name', async ({ run }) => {
    renderRun(run)
    expect(await screen.findByText(/3 inherited/)).toBeInTheDocument()
    expect(summary()).not.toHaveTextContent('none new')
  })
})
