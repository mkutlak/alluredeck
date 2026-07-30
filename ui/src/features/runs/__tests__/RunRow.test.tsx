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
  it('has data-testid="run-row"', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByTestId('run-row')).toBeInTheDocument()
  })

  it('renders the pipeline id and branch', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByText('196765')).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('falls back to the truncated SHA when there is no pipeline id', () => {
    renderWithProviders(<RunRow run={makeRun({ pipeline_id: undefined })} />)
    expect(screen.getByText('abc1234')).toBeInTheDocument()
  })

  it('links the group label to the numeric group_project_id', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByRole('link', { name: /acme/i })).toHaveAttribute('href', '/projects/10')
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
  it('is collapsed by default even when suites failed', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByTestId('run-row-toggle')).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
  })

  it('fetches no failures until the run is expanded', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(fetchRunFailures).not.toHaveBeenCalled()

    await user.click(screen.getByTestId('run-row-toggle'))
    expect(fetchRunFailures).toHaveBeenCalledTimes(1)
  })

  it('toggles the failure drawer', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RunRow run={makeRun()} />)
    const toggle = screen.getByTestId('run-row-toggle')

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(await screen.findByTestId('run-failures')).toBeInTheDocument()

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
  })

  it('shows only failing suites as chips, with the passing ones counted', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    const chips = screen.getAllByTestId('run-suite-chip')
    expect(chips).toHaveLength(1)
    expect(chips[0]).toHaveTextContent('ui-tests')
    expect(screen.getByText('· 1 passed')).toBeInTheDocument()
  })

  it('renders no chip row at all for a fully passing run', () => {
    renderWithProviders(<RunRow run={allPassingRun()} />)
    expect(screen.queryByTestId('run-suite-chips')).not.toBeInTheDocument()
  })

  it('summarises failing suites and total failures', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByText('1/2 suites failing · 15 failed tests')).toBeInTheDocument()
  })

  it('reports all suites passing without a failure count', () => {
    renderWithProviders(<RunRow run={allPassingRun()} />)
    expect(screen.getByText('1/1 suites passed')).toBeInTheDocument()
  })

  it('keeps the group link outside the toggle button', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByTestId('run-row-toggle')).not.toContainElement(
      screen.getByRole('link', { name: /acme/i }),
    )
  })
})
