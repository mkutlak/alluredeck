import { screen, within } from '@testing-library/react'
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
    data: [{ project_id: 10, slug: 'acme', display_name: 'Acme Payments', children: [1, 2, 3] }],
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

function suite(project_id: number, slug: string, failed: number): PipelineSuite {
  return {
    project_id,
    slug,
    build_number: 10 + project_id,
    build_id: 100 + project_id,
    pass_rate: 0,
    total: 100,
    passed: 100 - failed,
    failed,
    skipped: 0,
    duration_ms: 42000,
    status: failed > 0 ? 'degraded' : 'passed',
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
      suites_passed: suites.filter((s) => s.failed === 0).length,
      suites_total: suites.length,
      tests_passed: 200 - suites.reduce((n, s) => n + s.failed, 0),
      tests_total: 200,
      tests_skipped: 0,
      pass_rate: 97.5,
      total_duration_ms: 213000,
      awaiting_suites,
    },
  }
}

const passingRun = (awaiting?: number) =>
  makeRun([suite(1, 'api-cloud', 0), suite(2, 'ui-tests', 0)], awaiting)
const failingRun = (awaiting?: number) =>
  makeRun(
    [suite(1, 'api-cloud', 3), suite(2, 'ui-tests', 2), suite(3, 'contract-tests', 0)],
    awaiting,
  )

function failure(project_id: number, slug: string, test_name: string): RunFailure {
  return {
    project_id,
    slug,
    build_id: 100 + project_id,
    build_number: 10 + project_id,
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
  }
}

function renderRun(run: PipelineRun) {
  vi.mocked(fetchPipelineRun).mockResolvedValue({ data: run, metadata: { message: 'ok' } })
  const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
    initialEntries: ['/runs/10/48213'],
  })
  return renderWithProviders(<></>, { router })
}

const heading = () => screen.findByRole('heading', { level: 1 })
const summary = () => screen.getByTestId('run-page-summary')

beforeEach(() => {
  vi.mocked(fetchPipelineRun).mockReset()
  vi.mocked(fetchRunFailures).mockReset()
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: [
      ...['a1', 'a2', 'a3'].map((t) => failure(1, 'api-cloud', t)),
      ...['u1', 'u2'].map((t) => failure(2, 'ui-tests', t)),
    ],
    metadata: { message: 'ok', truncated: false },
  })
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

describe('RunPage for a pipeline run that is still uploading', () => {
  it('puts the neutral ○, named "in progress", beside the run ID instead of ✓', async () => {
    renderRun(passingRun(3))
    const h1 = await heading()

    expect(within(h1).getByRole('img', { name: /in progress/i })).toHaveTextContent('○')
    expect(h1).not.toHaveTextContent('✓')
    expect(h1).not.toHaveTextContent('✗')
    expect(h1.innerHTML).not.toMatch(/d20f39|f38ba8|40a02b|a6e3a1/)
  })

  it.each([
    { awaiting: 3, text: 'awaiting 3 suites' },
    { awaiting: 1, text: 'awaiting 1 suite' },
  ])('headlines $text with the facts that arrived, never a verdict', async ({ awaiting, text }) => {
    renderRun(passingRun(awaiting))
    await heading()

    expect(summary().textContent).toBe(`${text} · 100% passed · 3m 33s`)
    expect(summary()).not.toHaveTextContent(/suites? (passed|failing)/)
  })

  it('keeps the failed-test count and the failing suites when something failed', async () => {
    renderRun(failingRun(2))
    const h1 = await heading()

    expect(within(h1).getByRole('img', { name: /in progress/i })).toHaveTextContent('○')
    expect(summary()).toHaveTextContent(/^awaiting 2 suites · 5 failed tests · /)
    expect(summary()).not.toHaveTextContent(/suites? (passed|failing)/)
    expect(await screen.findAllByRole('region')).toHaveLength(2)
  })

  it.each([undefined, 0])(
    'keeps the usual wording when awaiting_suites is %s',
    async (awaiting) => {
      const { unmount } = renderRun(passingRun(awaiting))
      const h1 = await heading()
      expect(h1).toHaveAccessibleName('48213')
      expect(h1).toHaveTextContent('✓')
      expect(summary()).toHaveTextContent(/^2\/2 suites passed · /)
      unmount()

      renderRun(failingRun(awaiting))
      expect(await heading()).toHaveTextContent('✗')
      expect(summary()).toHaveTextContent(/^2\/3 suites failing · 5 failed tests/)
    },
  )
})
