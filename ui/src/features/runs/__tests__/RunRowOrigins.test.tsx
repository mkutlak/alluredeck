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
    ...overrides,
  }
}

function failingRun(origins?: PipelineAggregate['origins']): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      makeSuite(),
      makeSuite({ project_id: 2, slug: 'ui-tests', failed: 15, total: 100, status: 'degraded' }),
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

const summary = () => screen.getByTestId('run-row-summary')
// On the chips line, after "· N passed": the summary column keeps its width, so rows line up.
const origins = () => screen.queryByTestId('run-suite-chips-origins')

describe('RunRow origins of a failing run', () => {
  it('says where the failing tests came from on the chips line, not in the summary', () => {
    renderWithProviders(<RunRow run={failingRun({ new: 2, inherited: 3, environment: 1 })} />)

    expect(within(screen.getByTestId('run-suite-chips')).getByText('· 2 new')).toBeInTheDocument()
    expect(origins()).toHaveTextContent('Failing tests:· 2 new· 3 inherited (1 env)')
    expect(summary()).not.toHaveTextContent(/new|inherited/)
  })

  it('says nothing about origins when the API sent none', () => {
    renderWithProviders(<RunRow run={failingRun()} />)

    expect(origins()).not.toBeInTheDocument()
    expect(summary()).not.toHaveTextContent(/new|inherited/)
  })

  it('adds nothing when every origin is zero', () => {
    renderWithProviders(<RunRow run={failingRun({ new: 0, inherited: 0, environment: 0 })} />)

    expect(origins()).not.toBeInTheDocument()
  })
})
