import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineRun, PipelineSuite } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({ data: [], metadata: { message: 'ok' } }),
  getProjects: vi.fn(),
}))

function suite(overrides?: Partial<PipelineSuite>): PipelineSuite {
  return {
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
    ...overrides,
  }
}

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'feature/login',
    ci_build_url: 'https://ci.example.com/pipelines/196765',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [suite()],
    aggregate: {
      suites_passed: 0,
      suites_total: 1,
      tests_passed: 85,
      tests_total: 100,
      pass_rate: 85,
      total_duration_ms: 30000,
    },
    ...overrides,
  }
}

describe('RunRow links', () => {
  it.each([
    { name: 'its pipeline ID', run: makeRun(), href: '/runs/10/196765' },
    {
      name: 'its commit SHA when there is no pipeline ID',
      run: makeRun({ pipeline_id: undefined }),
      href: '/runs/10/abc1234def5678',
    },
    {
      name: 'an encoded key',
      run: makeRun({ pipeline_id: 'gh/run 7' }),
      href: '/runs/10/gh%2Frun%207',
    },
  ])('links the branch to the run page by $name', ({ run, href }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByRole('link', { name: /^feature\/login, run / })).toHaveAttribute(
      'href',
      href,
    )
  })

  // Every row has a "main" link; the run ID tells a screen reader which run each opens.
  it.each([
    { run: makeRun(), name: 'feature/login, run 196765, failed, 15 failed tests' },
    {
      run: makeRun({ pipeline_id: undefined }),
      name: 'feature/login, run abc1234, failed, 15 failed tests',
    },
    { run: makeRun({ branch: '' }), name: 'no branch, run 196765, failed, 15 failed tests' },
  ])('names the branch link "$name"', ({ run, name }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByRole('link', { name })).toBeInTheDocument()
  })

  // The pipeline ID stays the way out to CI.
  it('keeps the pipeline ID as the external CI link', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(
      screen.getByRole('link', { name: 'CI pipeline 196765 (opens in new tab)' }),
    ).toHaveAttribute('href', 'https://ci.example.com/pipelines/196765')
  })

  it('offers "no branch" as the link text when the run has no branch', () => {
    renderWithProviders(<RunRow run={makeRun({ branch: '' })} />)
    expect(screen.getByRole('link', { name: /^no branch/ })).toHaveAttribute(
      'href',
      '/runs/10/196765',
    )
  })

  // No group, no run page to open: the branch stays plain text.
  it('does not link the branch of a run without a group project', () => {
    renderWithProviders(<RunRow run={makeRun({ group_project_id: undefined })} />)
    expect(screen.getByText('feature/login')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /feature\/login/ })).not.toBeInTheDocument()
  })

  it.each([
    { name: 'single build', builds: undefined },
    {
      name: 'sharded',
      builds: [1, 2, 3].map((n) => ({ build_id: 100 + n, build_number: n })),
    },
  ])('points a $name suite chip at its section of the run page', ({ builds }) => {
    renderWithProviders(<RunRow run={makeRun({ suites: [suite(builds ? { builds } : {})] })} />)
    expect(screen.getByRole('link', { name: /^ui-tests/ })).toHaveAttribute(
      'href',
      '/runs/10/196765#suite-2',
    )
  })
})
