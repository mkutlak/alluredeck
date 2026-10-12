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

function makeRun(
  overrides?: Partial<PipelineRun>,
  aggregate?: Partial<PipelineRun['aggregate']>,
): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'feature/login',
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
      ...aggregate,
    },
    ...overrides,
  }
}

const passed = suite({ failed: 0, status: 'passed' })
const skipped = suite({ failed: 0, status: 'skipped' })
const head = 'feature/login, run 196765'

// A screen reader jumping by heading hears the verdict, in the words the row shows.
describe('RunRow heading link carries the verdict', () => {
  it.each([
    { name: 'failed', run: makeRun(), label: `${head}, failed, 15 failed tests` },
    {
      name: 'one failed test',
      run: makeRun({ suites: [suite({ failed: 1 })] }),
      label: `${head}, failed, 1 failed test`,
    },
    {
      name: 'failed with new failures',
      run: makeRun({}, { origins: { new: 2, inherited: 13, environment: 0, unknown: 0 } }),
      label: `${head}, failed, 15 failed tests, 2 new`,
    },
    {
      name: 'failed, none new',
      run: makeRun({}, { origins: { new: 0, inherited: 15, environment: 0, unknown: 0 } }),
      label: `${head}, failed, 15 failed tests, none new`,
    },
    {
      name: 'failed, inherited only from an API that sends no unknown count',
      run: makeRun({}, { origins: { new: 0, inherited: 15, environment: 0 } }),
      label: `${head}, failed, 15 failed tests`,
    },
    {
      name: 'passed',
      run: makeRun({ suites: [passed] }, { suites_passed: 1 }),
      label: `${head}, passed`,
    },
    {
      name: 'all skipped',
      run: makeRun({ suites: [skipped] }),
      label: `${head}, all tests skipped`,
    },
    {
      name: 'awaiting suites',
      run: makeRun({ suites: [passed] }, { suites_passed: 1, awaiting_suites: 3 }),
      label: `${head}, awaiting 3 suites`,
    },
    {
      name: 'awaiting one suite',
      run: makeRun({ suites: [passed] }, { suites_passed: 1, awaiting_suites: 1 }),
      label: `${head}, awaiting 1 suite`,
    },
    {
      name: 'awaiting, with failures so far',
      run: makeRun({ suites: [suite({ failed: 2 })] }, { awaiting_suites: 3 }),
      label: `${head}, awaiting 3 suites, 2 failed tests`,
    },
    {
      name: 'failed with an unreadable suite',
      run: makeRun({ suites: [suite({ failed: 0, status: 'failed' })] }),
      label: `${head}, failed, 1 suite failing`,
    },
    {
      name: 'awaiting, with an unreadable suite so far',
      run: makeRun({ suites: [suite({ failed: 0, status: 'failed' })] }, { awaiting_suites: 3 }),
      label: `${head}, awaiting 3 suites, 1 suite failing`,
    },
    {
      name: 'no branch',
      run: makeRun({ branch: '', suites: [passed] }),
      label: 'no branch, run 196765, passed',
    },
  ])('names a run that is $name', ({ run, label }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByRole('link', { name: label })).toHaveAttribute('href', '/runs/10/196765')
  })
})

describe('RunRow CI link', () => {
  // RunRowLinks covers the link's name with ci_build_url alone.
  it('prefers pipeline_url over ci_build_url', () => {
    renderWithProviders(
      <RunRow
        run={makeRun({
          pipeline_url: 'https://ci.example.com/p/1',
          ci_build_url: 'https://ci.example.com/b/1',
        })}
      />,
    )
    expect(screen.getByRole('link', { name: /^CI pipeline / })).toHaveAttribute(
      'href',
      'https://ci.example.com/p/1',
    )
  })
})
