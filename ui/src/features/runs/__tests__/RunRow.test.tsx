import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineRun } from '@/types/api'

vi.mock('@/api/builds', () => ({
  fetchBuildFailedTests: vi.fn().mockResolvedValue([]),
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

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    commit_sha: 'abc1234def5678',
    branch: 'main',
    ci_build_url: 'https://ci.example.com/pipelines/123',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      {
        project_id: 1,
        slug: 'api-cloud',
        build_number: 5,
        build_id: 105,
        pass_rate: 100,
        total: 42,
        failed: 0,
        duration_ms: 15000,
        status: 'passed',
      },
      {
        project_id: 2,
        slug: 'ui-tests',
        build_number: 3,
        build_id: 103,
        pass_rate: 85,
        total: 100,
        failed: 15,
        duration_ms: 30000,
        status: 'degraded',
      },
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

describe('RunRow', () => {
  it('has data-testid="run-row"', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByTestId('run-row')).toBeInTheDocument()
  })

  it('renders truncated SHA and branch badge', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByText('abc1234')).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('renders suite chips for every suite, always visible', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getAllByTestId('run-suite-chip')).toHaveLength(2)
  })

  it('links the group label to the numeric group_project_id', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    const link = screen.getByRole('link', { name: /acme/i })
    expect(link).toHaveAttribute('href', '/projects/10')
  })

  it('auto-expands when there are suite failures (suites_passed < suites_total)', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    const button = screen.getByRole('button')
    expect(button).toHaveAttribute('aria-expanded', 'true')
  })

  it('does NOT auto-expand when all suites pass', () => {
    const run = makeRun({
      aggregate: {
        suites_passed: 2,
        suites_total: 2,
        tests_passed: 142,
        tests_total: 142,
        pass_rate: 100,
        total_duration_ms: 45000,
      },
    })
    renderWithProviders(<RunRow run={run} />)
    const button = screen.getByRole('button')
    expect(button).toHaveAttribute('aria-expanded', 'false')
  })

  it('toggles expansion when the header button is clicked', async () => {
    const user = userEvent.setup()
    const run = makeRun({
      aggregate: {
        suites_passed: 2,
        suites_total: 2,
        tests_passed: 142,
        tests_total: 142,
        pass_rate: 100,
        total_duration_ms: 45000,
      },
    })
    renderWithProviders(<RunRow run={run} />)
    const button = screen.getByRole('button')
    expect(button).toHaveAttribute('aria-expanded', 'false')
    await user.click(button)
    expect(button).toHaveAttribute('aria-expanded', 'true')
  })

  it('falls back to group_slug when group_project_id is absent', () => {
    renderWithProviders(<RunRow run={makeRun({ group_project_id: undefined, group_slug: 'orphan-group' })} />)
    expect(screen.getByText('orphan-group')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /orphan-group/i })).not.toBeInTheDocument()
  })
})
