import { screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineAggregate, PipelineRun, PipelineSuite } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
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

function suite(overrides?: Partial<PipelineSuite>): PipelineSuite {
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
    ...overrides,
  }
}

// One passing suite, one failing.
function failingRun(origins: PipelineAggregate['origins']): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      suite(),
      suite({ project_id: 2, slug: 'ui-tests', failed: 15, total: 100, status: 'degraded' }),
    ],
    aggregate: {
      suites_passed: 1,
      suites_total: 2,
      tests_passed: 127,
      tests_total: 142,
      pass_rate: 89.4,
      total_duration_ms: 45000,
      origins,
    },
  }
}

const chips = () => within(screen.getByTestId('run-suite-chips'))
const originsLine = () => screen.getByTestId('run-suite-chips-origins')

// "2 new" must not read as the whole answer when some failures cannot be placed, and
// the part that answers "is it me?" leads the line.
describe('RunRow origin words on the chips line', () => {
  it('counts failures of unknown origin', () => {
    renderWithProviders(
      <RunRow run={failingRun({ new: 2, inherited: 0, environment: 0, unknown: 1 })} />,
    )
    expect(chips().getByText('· 1 unknown')).toBeInTheDocument()
  })

  it('says "none new" when every failure is inherited', () => {
    renderWithProviders(
      <RunRow run={failingRun({ new: 0, inherited: 3, environment: 1, unknown: 0 })} />,
    )
    expect(screen.getByTestId('run-suite-chips-origins')).toHaveTextContent(
      'Failing tests:· none new· 3 inherited (1 env)',
    )
  })

  it('sets the "new" part in ink, apart from the other counts', () => {
    renderWithProviders(
      <RunRow run={failingRun({ new: 2, inherited: 3, environment: 0, unknown: 0 })} />,
    )
    expect(chips().getByText('· 2 new')).toHaveClass('text-foreground', 'font-medium')
  })

  it('says the passed count is of suites, in the singular for one', () => {
    renderWithProviders(
      <RunRow run={failingRun({ new: 2, inherited: 0, environment: 0, unknown: 0 })} />,
    )
    expect(chips().getByText('· 1 suite passed')).toBeInTheDocument()
  })

  it('sets "none new" in ink, as it answers "is it me?" like "2 new" does', () => {
    renderWithProviders(
      <RunRow run={failingRun({ new: 0, inherited: 3, environment: 0, unknown: 0 })} />,
    )
    expect(chips().getByText('· none new')).toHaveClass('text-foreground', 'font-medium')
  })

  it('leaves the "inherited" and "unknown" parts in secondary ink', () => {
    renderWithProviders(
      <RunRow run={failingRun({ new: 2, inherited: 3, environment: 0, unknown: 1 })} />,
    )
    for (const part of ['· 3 inherited', '· 1 unknown']) {
      expect(chips().getByText(part)).toHaveClass('text-fact')
      expect(chips().getByText(part)).not.toHaveClass('text-foreground')
    }
  })

  // "none new" claims every failure was placed; these runs cannot back that.
  describe('does not say "none new"', () => {
    const inheritedOnly = { new: 0, inherited: 3, environment: 0, unknown: 0 }
    const withSuites = (run: PipelineRun, suites: PipelineSuite[]): PipelineRun => ({
      ...run,
      suites: [...run.suites, ...suites],
    })

    it('when the API sends no unknown count (an older API that drops unknowns)', () => {
      renderWithProviders(<RunRow run={failingRun({ new: 0, inherited: 3, environment: 0 })} />)
      expect(originsLine()).toHaveTextContent('Failing tests:· 3 inherited')
      expect(originsLine()).not.toHaveTextContent('none new')
    })

    it('while suites are still uploading', () => {
      const run = failingRun(inheritedOnly)
      renderWithProviders(
        <RunRow run={{ ...run, aggregate: { ...run.aggregate, awaiting_suites: 2 } }} />,
      )
      expect(originsLine()).toHaveTextContent('Failing tests:· 3 inherited')
      expect(originsLine()).not.toHaveTextContent('none new')
    })

    it('when a suite failed without a failing test (its report was unreadable)', () => {
      renderWithProviders(
        <RunRow
          run={withSuites(failingRun(inheritedOnly), [
            suite({ project_id: 3, slug: 'broken', status: 'failed', failed: 0, total: 0 }),
          ])}
        />,
      )
      expect(originsLine()).toHaveTextContent('Failing tests:· 3 inherited')
      expect(originsLine()).not.toHaveTextContent('none new')
    })
  })

  // Mirrors the API's suites_passed: only a suite whose status is "passed" counts.
  it.each([
    { name: 'an unreadable', extra: suite({ project_id: 3, status: 'failed', failed: 0 }) },
    { name: 'an all-skipped', extra: suite({ project_id: 3, status: 'skipped', failed: 0 }) },
  ])('does not count $name suite as passed', ({ extra }) => {
    const run = failingRun({ new: 2, inherited: 0, environment: 0, unknown: 0 })
    renderWithProviders(<RunRow run={{ ...run, suites: [...run.suites, extra] }} />)
    expect(chips().getByText('· 1 suite passed')).toBeInTheDocument()
  })
})
