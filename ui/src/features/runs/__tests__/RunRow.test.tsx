import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineRun, PipelineSuite } from '@/types/api'

const fetchRunFailures = vi.fn().mockResolvedValue({
  data: [],
  metadata: { message: 'ok', truncated: false },
})

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: (...args: unknown[]) => fetchRunFailures(...args),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 10, slug: 'acme', display_name: 'Acme', parent_id: null, children: [1, 2] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

function makeSuite(overrides?: Partial<PipelineSuite>): PipelineSuite {
  return {
    project_id: 1,
    slug: 'api-cloud',
    build_number: 5,
    build_id: 105,
    pass_rate: 100,
    total: 42,
    failed: 0,
    duration_ms: 15000,
    status: 'passed',
    builds: [{ build_id: 105, build_number: 5 }],
    ...overrides,
  }
}

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    ci_build_url: 'https://ci.example.com/pipelines/123',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      makeSuite(),
      makeSuite({
        project_id: 2,
        slug: 'ui-tests',
        build_number: 3,
        build_id: 103,
        pass_rate: 85,
        total: 100,
        failed: 15,
        duration_ms: 30000,
        status: 'degraded',
        builds: [{ build_id: 103, build_number: 3 }],
      }),
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 2,
      tests_passed: 127,
      tests_total: 142,
      pass_rate: 89.4,
      total_duration_ms: 45000,
    },
    ...overrides,
  }
}

function allPassingRun(): PipelineRun {
  return makeRun({
    suites: [makeSuite()],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
    },
  })
}

beforeEach(() => {
  fetchRunFailures.mockClear()
})

describe('RunRow', () => {
  it.each([
    { run: makeRun(), id: '196765' },
    // No pipeline id: fall back to the truncated SHA.
    { run: makeRun({ pipeline_id: undefined }), id: 'abc1234' },
  ])('identifies the run by $id, with its branch', ({ run, id }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByText(id)).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('links the group label to the numeric group_project_id, outside the toggle button', async () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    const link = await screen.findByRole('link', { name: /acme/i })
    expect(link).toHaveAttribute('href', '/projects/10')
    expect(screen.getByTestId('run-row-toggle')).not.toContainElement(link)
  })

  it('falls back to group_slug when group_project_id is absent', () => {
    renderWithProviders(
      <RunRow run={makeRun({ group_project_id: undefined, group_slug: 'orphan-group' })} />,
    )
    expect(screen.getByText('orphan-group')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /orphan-group/i })).not.toBeInTheDocument()
  })

  // Auto-expanding every failing run is what made a page of ten runs
  // unscannable, and it fetched failures nobody had asked to see.
  it('is collapsed without fetching failures until toggled open, and toggles closed', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RunRow run={makeRun()} />)
    const toggle = screen.getByTestId('run-row-toggle')

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
    expect(fetchRunFailures).not.toHaveBeenCalled()

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(await screen.findByTestId('run-failures')).toBeInTheDocument()
    expect(fetchRunFailures).toHaveBeenCalledTimes(1)

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
  })

  it.each([
    {
      // Failing suites become chips; a green run gets none.
      run: makeRun(),
      summary: '1/2 suites failing · 15 failed tests',
      chips: ['ui-tests'],
    },
    {
      run: allPassingRun(),
      summary: '1/1 suites passed',
      chips: [],
    },
  ])('summarises the run as $summary with chips $chips', ({ run, summary, chips }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByText(summary)).toBeInTheDocument()
    expect(screen.queryAllByTestId('run-suite-chip').map((c) => c.textContent)).toEqual(
      chips.map((c) => expect.stringContaining(c)),
    )
  })
})
