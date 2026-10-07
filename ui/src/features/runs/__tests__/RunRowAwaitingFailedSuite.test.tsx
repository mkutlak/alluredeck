import { screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunPage } from '../RunPage'
import { RunRow } from '../RunRow'
import type { PipelineRun, PipelineSuite } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRun: vi.fn(),
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', display_name: 'Acme', children: [1, 2, 3] }],
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

// A suite whose report could not be read arrives `failed` with no counts, so it fails
// the run while no test is listed as failed.
function unreadable(project_id: number): PipelineSuite {
  return {
    project_id,
    slug: `unreadable-${project_id}`,
    build_number: 1,
    build_id: 100 + project_id,
    pass_rate: 0,
    total: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    duration_ms: 1000,
    status: 'failed',
  }
}

function suite(project_id: number, failed: number): PipelineSuite {
  return {
    ...unreadable(project_id),
    slug: `suite-${project_id}`,
    total: 100,
    passed: 100 - failed,
    failed,
    status: failed > 0 ? 'degraded' : 'passed',
  }
}

function makeRun(suites: PipelineSuite[], awaiting_suites = 2): PipelineRun {
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
      tests_passed: 100,
      tests_total: 100,
      pass_rate: 100,
      total_duration_ms: 213000,
      awaiting_suites,
    },
  }
}

const rowSummary = () => screen.getByTestId('run-row-summary')

function renderPage(run: PipelineRun) {
  vi.mocked(fetchPipelineRun).mockResolvedValue({ data: run, metadata: { message: 'ok' } })
  const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
    initialEntries: ['/runs/10/48213'],
  })
  return renderWithProviders(<></>, { router })
}

beforeEach(() => {
  vi.mocked(fetchPipelineRun).mockReset()
  vi.mocked(fetchRunFailures).mockReset()
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: [],
    metadata: { message: 'ok', truncated: false },
  })
})

describe('an awaiting run with a failing suite that lists no failed test', () => {
  it('keeps the failing suite on the row, as a red count without a denominator', () => {
    renderWithProviders(<RunRow run={makeRun([unreadable(1), suite(2, 0)])} />)

    expect(rowSummary().textContent).toBe('awaiting 2 suites · 1 suite failing')
    expect(screen.getByText('1 suite failing')).toHaveClass('text-[#d20f39]')
    expect(rowSummary()).not.toHaveTextContent(/\d\/\d/)
    // Still no verdict: the glyph is the neutral ○.
    expect(screen.getByRole('img', { name: /in progress/i })).toHaveTextContent('○')
  })

  it('says "2 suites failing" for two of them', () => {
    renderWithProviders(<RunRow run={makeRun([unreadable(1), unreadable(2)], 1)} />)
    expect(rowSummary().textContent).toBe('awaiting 1 suite · 2 suites failing')
  })

  it('shows the failed-test count alone when there is one, not both', () => {
    renderWithProviders(<RunRow run={makeRun([unreadable(1), suite(2, 15)])} />)
    expect(rowSummary().textContent).toBe('awaiting 2 suites · 15 failed tests')
  })

  it('shows no failing count when nothing that arrived failed', () => {
    renderWithProviders(<RunRow run={makeRun([suite(1, 0), suite(2, 0)])} />)
    expect(rowSummary().textContent).toBe('awaiting 2 suites')
  })

  it('keeps it on the run page header too', async () => {
    renderPage(makeRun([unreadable(1), suite(2, 0)]))
    await screen.findByRole('heading', { level: 1 })

    expect(screen.getByTestId('run-page-summary')).toHaveTextContent(
      /^awaiting 2 suites · 1 suite failing · /,
    )
    expect(screen.getByText('1 suite failing')).toHaveClass('text-[#d20f39]')
  })

  it('shows the failed-test count alone on the run page header when there is one', async () => {
    renderPage(makeRun([unreadable(1), suite(2, 5)]))
    await screen.findByRole('heading', { level: 1 })

    expect(screen.getByTestId('run-page-summary')).toHaveTextContent(
      /^awaiting 2 suites · 5 failed tests · /,
    )
    expect(screen.getByTestId('run-page-summary')).not.toHaveTextContent(/suites? failing/)
  })
})
