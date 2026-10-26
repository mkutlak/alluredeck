import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import { RunSuiteChips } from '../RunSuiteChips'
import type { PipelineRun, PipelineSuite } from '@/types/api'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({ data: [], metadata: { message: 'ok' } }),
  getProjects: vi.fn(),
}))

function makeSuite(overrides: Partial<PipelineSuite> & { project_id: number }): PipelineSuite {
  return {
    slug: `suite-${overrides.project_id}`,
    build_number: 1,
    build_id: 1,
    pass_rate: 100,
    total: 10,
    failed: 0,
    duration_ms: 1000,
    status: 'passed',
    builds: [{ build_id: 1, build_number: 1 }],
    ...overrides,
  }
}

// A suite whose report could not be read has no counts, but it still failed its run.
const unreadable = makeSuite({
  project_id: 1,
  slug: 'broken-report',
  total: 0,
  pass_rate: 0,
  status: 'failed',
})

describe('RunSuiteChips with a suite whose report could not be read', () => {
  // The row draws its chips line for a failing run, not only for one with failed tests.
  it('renders a chip for it on the feed row when it is the only failing suite', () => {
    const suites = [unreadable, makeSuite({ project_id: 2 })]
    const run: PipelineRun = {
      pipeline_id: '48213',
      commit_sha: 'a1b2c3d4e5f6',
      branch: 'main',
      timestamp: '2026-10-07T09:12:00Z',
      group_project_id: 10,
      group_slug: 'acme',
      suites,
      aggregate: {
        suites_passed: 1,
        suites_total: 2,
        tests_passed: 10,
        tests_total: 10,
        pass_rate: 100,
        total_duration_ms: 1000,
      },
    }
    renderWithProviders(<RunRow run={run} />)

    expect(screen.getAllByTestId('run-suite-chip')).toHaveLength(1)
    expect(screen.getByText('broken-report')).toBeInTheDocument()
    expect(screen.getByText('· 1 suite passed')).toBeInTheDocument()
  })

  it('renders it beside a suite with failed tests', () => {
    renderWithProviders(
      <RunSuiteChips
        suites={[unreadable, makeSuite({ project_id: 2, slug: 'degraded', failed: 3 })]}
      />,
    )

    expect(screen.getAllByTestId('run-suite-chip')).toHaveLength(2)
  })

  it('names it as failed, since it has no count to say so', () => {
    renderWithProviders(<RunSuiteChips suites={[unreadable]} />)

    expect(screen.getByRole('link', { name: 'broken-report: failed' })).toBeInTheDocument()
  })
})
