import { screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import type { BuildFailedTest, PipelineRun } from '@/types/api'

vi.mock('@/api/builds', () => ({
  fetchBuildFailedTests: vi.fn(),
}))

import { fetchBuildFailedTests } from '@/api/builds'

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
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

function makeTest(overrides?: Partial<BuildFailedTest>): BuildFailedTest {
  return {
    test_name: 'should login',
    full_name: 'suite.should login',
    status: 'failed',
    duration_ms: 120,
    history_id: 'h1',
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'Expected true to be false\n    at line 12',
    ...overrides,
  }
}

describe('RunFailures', () => {
  beforeEach(() => {
    vi.mocked(fetchBuildFailedTests).mockReset()
  })

  it('fetches only suites with failed > 0', async () => {
    vi.mocked(fetchBuildFailedTests).mockResolvedValue([])
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(fetchBuildFailedTests).toHaveBeenCalledTimes(1)
    })
    expect(fetchBuildFailedTests).toHaveBeenCalledWith(2, 103)
  })

  it('renders failed test rows with suite slug', async () => {
    vi.mocked(fetchBuildFailedTests).mockResolvedValue([makeTest()])
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(screen.getByText('should login')).toBeInTheDocument()
    })
    expect(screen.getByText('ui-tests')).toBeInTheDocument()
  })

  it('displays only the first line of a multi-line error message, with full text in title', async () => {
    vi.mocked(fetchBuildFailedTests).mockResolvedValue([makeTest()])
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(screen.getByText('Expected true to be false')).toBeInTheDocument()
    })
    const el = screen.getByText('Expected true to be false')
    expect(el).toHaveAttribute('title', 'Expected true to be false\n    at line 12')
  })

  it('shows badges for flaky/new/known tests', async () => {
    vi.mocked(fetchBuildFailedTests).mockResolvedValue([
      makeTest({ flaky: true, new_failed: true, known: true }),
    ])
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(screen.getByText('flaky')).toBeInTheDocument()
    })
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(screen.getByText('known')).toBeInTheDocument()
  })

  it('links to the suite report using numeric project_id and build_number', async () => {
    vi.mocked(fetchBuildFailedTests).mockResolvedValue([makeTest()])
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(screen.getByText('ui-tests')).toBeInTheDocument()
    })
    expect(screen.getByRole('link', { name: 'ui-tests' })).toHaveAttribute(
      'href',
      '/projects/2/reports/3',
    )
  })

  it('renders nothing when no suites have failures', () => {
    const run = makeRun({
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
      ],
    })
    renderWithProviders(<RunFailures run={run} />)
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
    expect(fetchBuildFailedTests).not.toHaveBeenCalled()
  })

  it('shows an error state when a query fails', async () => {
    vi.mocked(fetchBuildFailedTests).mockRejectedValue(new Error('network error'))
    renderWithProviders(<RunFailures run={makeRun()} />)

    expect(await screen.findByText(/failed to load/i)).toBeInTheDocument()
  })
})
