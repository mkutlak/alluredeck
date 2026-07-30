import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type {
  ApiResponse,
  ConfigData,
  PipelineRun,
  PipelineSuite,
  RunFailure,
  RunFailuresResponse,
} from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi.fn(),
}))

import { fetchRunFailures } from '@/api/pipeline'
import { fetchFailureSummary } from '@/api/failures'
import { getConfig } from '@/api/system'

function makeConfig(overrides?: Partial<ConfigData>): ApiResponse<ConfigData> {
  return {
    data: { llm_enabled: true, ...overrides } as ConfigData,
    metadata: { message: 'ok' },
  }
}

function makeSuite(overrides?: Partial<PipelineSuite>): PipelineSuite {
  return {
    project_id: 2,
    slug: 'ui-tests',
    build_number: 3,
    build_id: 103,
    pass_rate: 85,
    total: 100,
    failed: 2,
    duration_ms: 30000,
    status: 'degraded',
    builds: [{ build_id: 103, build_number: 3 }],
    ...overrides,
  }
}

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [makeSuite()],
    aggregate: {
      suites_passed: 0,
      suites_total: 1,
      tests_passed: 98,
      tests_total: 100,
      pass_rate: 98,
      total_duration_ms: 30000,
    },
    ...overrides,
  }
}

function makeFailure(overrides?: Partial<RunFailure>): RunFailure {
  return {
    project_id: 2,
    slug: 'ui-tests',
    build_id: 103,
    build_number: 3,
    test_name: 'should login',
    full_name: 'login.spec.js:1:1',
    status: 'failed',
    duration_ms: 100,
    history_id: 'h1',
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'TimeoutError: locator.click',
    ...overrides,
  }
}

function respond(failures: RunFailure[], truncated = false): RunFailuresResponse {
  return { data: failures, metadata: { message: 'ok', truncated } }
}

beforeEach(() => {
  vi.mocked(fetchRunFailures).mockReset()
  vi.mocked(fetchFailureSummary).mockReset()
  vi.mocked(getConfig).mockReset()
  vi.mocked(getConfig).mockResolvedValue(makeConfig({ llm_enabled: false }))
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

describe('RunFailures', () => {
  it('fetches the whole run in a single request', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(screen.getByText('should login')).toBeInTheDocument()
    })
    expect(fetchRunFailures).toHaveBeenCalledTimes(1)
    expect(fetchRunFailures).toHaveBeenCalledWith(10, '196765')
  })

  it('keys the request by commit SHA when the run has no pipeline id', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(respond([]))
    renderWithProviders(<RunFailures run={makeRun({ pipeline_id: undefined })} />)

    await waitFor(() => {
      expect(fetchRunFailures).toHaveBeenCalledWith(10, 'abc1234')
    })
  })

  it('renders an error state when the request fails', async () => {
    vi.mocked(fetchRunFailures).mockRejectedValue(new Error('network error'))
    renderWithProviders(<RunFailures run={makeRun()} />)

    expect(await screen.findByText(/failed to load failures/i)).toBeInTheDocument()
  })

  it('reports when a run has no failing tests', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(respond([]))
    renderWithProviders(<RunFailures run={makeRun()} />)

    expect(await screen.findByText(/no failing tests in this run/i)).toBeInTheDocument()
  })

  it('does not request anything when the run has no group project', () => {
    renderWithProviders(<RunFailures run={makeRun({ group_project_id: undefined })} />)
    expect(fetchRunFailures).not.toHaveBeenCalled()
    expect(screen.getByText(/failure details are unavailable/i)).toBeInTheDocument()
  })

  it('shows badges for flaky, new and known failures', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(
      respond([makeFailure({ flaky: true, retries: 2, new_failed: true, known: true })]),
    )
    renderWithProviders(<RunFailures run={makeRun()} />)

    expect(await screen.findByTestId('flaky-badge')).toBeInTheDocument()
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(screen.getByText('known')).toBeInTheDocument()
  })

  it('links a failure to its suite report by numeric project id and build number', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
    renderWithProviders(<RunFailures run={makeRun()} />)

    const link = await screen.findByRole('link', { name: 'should login' })
    expect(link).toHaveAttribute('href', '/projects/2/reports/3')
  })

  it('notes when the API truncated the result', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()], true))
    renderWithProviders(<RunFailures run={makeRun()} />)

    expect(await screen.findByText(/this run has more/i)).toBeInTheDocument()
  })

  // The same test arrives twice with different history_ids because two
  // ingestion paths write it; only one copy carries the error message.
  it('collapses duplicate copies of one test into a single row', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue(
      respond([
        makeFailure({ history_id: '1ab6c50a.d93c', retries: 3, error_message: '' }),
        makeFailure({ history_id: '462170f6:d93c', retries: 0 }),
      ]),
    )
    renderWithProviders(<RunFailures run={makeRun()} />)

    await waitFor(() => {
      expect(screen.getAllByTestId('run-failure-row')).toHaveLength(1)
    })
    expect(screen.getByText('1 failure · 1 suite')).toBeInTheDocument()
  })

  describe('grouping', () => {
    const twoSuites = [
      makeFailure({ test_name: 'a', full_name: 'a.js:1:1' }),
      makeFailure({ test_name: 'b', full_name: 'b.js:1:1' }),
      makeFailure({
        project_id: 3,
        slug: 'api-tests',
        test_name: 'c',
        full_name: 'c.js:1:1',
        error_message: 'Failed to execute query',
      }),
    ]

    it('groups by suite by default', async () => {
      vi.mocked(fetchRunFailures).mockResolvedValue(respond(twoSuites))
      renderWithProviders(<RunFailures run={makeRun()} />)

      await waitFor(() => {
        expect(screen.getAllByTestId('run-failure-group')).toHaveLength(2)
      })
      expect(screen.getByText('ui-tests')).toBeInTheDocument()
      expect(screen.getByText('api-tests')).toBeInTheDocument()
    })

    it('opens only the first group so the drawer stays short', async () => {
      vi.mocked(fetchRunFailures).mockResolvedValue(respond(twoSuites))
      renderWithProviders(<RunFailures run={makeRun()} />)

      // ui-tests has two failures and sorts first; its rows show, api-tests' do not.
      await waitFor(() => {
        expect(screen.getByText('a')).toBeInTheDocument()
      })
      expect(screen.queryByText('c')).not.toBeInTheDocument()
    })

    it('expands a collapsed group on click', async () => {
      const user = userEvent.setup()
      vi.mocked(fetchRunFailures).mockResolvedValue(respond(twoSuites))
      renderWithProviders(<RunFailures run={makeRun()} />)

      await waitFor(() => {
        expect(screen.getByText('api-tests')).toBeInTheDocument()
      })
      await user.click(screen.getByText('api-tests'))
      expect(screen.getByText('c')).toBeInTheDocument()
    })

    // 18+ failures per run routinely share one message differing only in a
    // timeout value; by-error states that once.
    it('clusters failures by normalised error signature', async () => {
      const user = userEvent.setup()
      vi.mocked(fetchRunFailures).mockResolvedValue(
        respond([
          makeFailure({
            test_name: 'a',
            full_name: 'a.js:1:1',
            error_message: 'Timed out 5000ms waiting for expect(locator)',
          }),
          makeFailure({
            test_name: 'b',
            full_name: 'b.js:1:1',
            error_message: 'Timed out 10000ms waiting for expect(locator)',
          }),
          makeFailure({
            test_name: 'c',
            full_name: 'c.js:1:1',
            error_message: 'Failed to execute query',
          }),
        ]),
      )
      renderWithProviders(<RunFailures run={makeRun()} />)

      await waitFor(() => {
        expect(screen.getByTestId('run-failure-grouping')).toBeInTheDocument()
      })
      await user.click(screen.getByTestId('run-failure-grouping-error'))

      await waitFor(() => {
        expect(screen.getAllByTestId('run-failure-group')).toHaveLength(2)
      })
      expect(screen.getByText('Timed out Nms waiting for expect(locator)')).toBeInTheDocument()
      expect(screen.getByText('2 tests · 1 suite')).toBeInTheDocument()
    })

    it('persists the grouping choice in the UI store', async () => {
      const user = userEvent.setup()
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      renderWithProviders(<RunFailures run={makeRun()} />)

      await waitFor(() => {
        expect(screen.getByTestId('run-failure-grouping')).toBeInTheDocument()
      })
      await user.click(screen.getByTestId('run-failure-grouping-error'))

      expect(useUIStore.getState().runsFailureGrouping).toBe('error')
    })
  })

  describe('sharded suites', () => {
    it('links every contributing build from the group header', async () => {
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      const run = makeRun({
        suites: [
          makeSuite({
            builds: [
              { build_id: 101, build_number: 1 },
              { build_id: 102, build_number: 2 },
              { build_id: 103, build_number: 3 },
            ],
          }),
        ],
      })
      renderWithProviders(<RunFailures run={run} />)

      await waitFor(() => {
        expect(screen.getByText('3 shards')).toBeInTheDocument()
      })
      expect(screen.getByRole('link', { name: /#1/ })).toHaveAttribute(
        'href',
        '/projects/2/reports/1',
      )
      expect(screen.getByRole('link', { name: /#3/ })).toHaveAttribute(
        'href',
        '/projects/2/reports/3',
      )
    })
  })

  describe('AI failure summary', () => {
    it('shows a collapsed toggle per failure when llm is enabled', async () => {
      vi.mocked(getConfig).mockResolvedValue(makeConfig())
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      renderWithProviders(<RunFailures run={makeRun()} />)

      const toggle = await screen.findByRole('button', { name: /toggle ai failure summary/i })
      expect(toggle).toHaveAttribute('aria-expanded', 'false')
    })

    it('does not render the toggle when llm_enabled is false', async () => {
      vi.mocked(getConfig).mockResolvedValue(makeConfig({ llm_enabled: false }))
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      renderWithProviders(<RunFailures run={makeRun()} />)

      await waitFor(() => {
        expect(screen.getByText('should login')).toBeInTheDocument()
      })
      expect(
        screen.queryByRole('button', { name: /toggle ai failure summary/i }),
      ).not.toBeInTheDocument()
    })

    it('does not render the toggle while config is still resolving', async () => {
      vi.mocked(getConfig).mockReturnValue(new Promise(() => {}))
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      renderWithProviders(<RunFailures run={makeRun()} />)

      await waitFor(() => {
        expect(screen.getByText('should login')).toBeInTheDocument()
      })
      expect(
        screen.queryByRole('button', { name: /toggle ai failure summary/i }),
      ).not.toBeInTheDocument()
    })

    it('mounts the summary panel with the failure build and history id', async () => {
      const user = userEvent.setup()
      vi.mocked(getConfig).mockResolvedValue(makeConfig())
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      vi.mocked(fetchFailureSummary).mockResolvedValue({
        enabled: true,
        summary: {
          hypothesis: 'Looks like a stale selector.',
          category: 'test_bug',
          evidence: ['Element not found'],
        },
        disclaimer: 'AI hypothesis — verify before acting.',
      })
      renderWithProviders(<RunFailures run={makeRun()} />)

      await user.click(await screen.findByRole('button', { name: /toggle ai failure summary/i }))

      await waitFor(() => {
        expect(fetchFailureSummary).toHaveBeenCalledWith(2, 103, 'h1')
      })
      expect(await screen.findByText('AI hypothesis')).toBeInTheDocument()
    })

    it('collapses the panel again on a second click', async () => {
      const user = userEvent.setup()
      vi.mocked(getConfig).mockResolvedValue(makeConfig())
      vi.mocked(fetchRunFailures).mockResolvedValue(respond([makeFailure()]))
      vi.mocked(fetchFailureSummary).mockResolvedValue({
        enabled: true,
        summary: { hypothesis: 'Stale selector.', category: 'test_bug', evidence: [] },
        disclaimer: 'AI hypothesis — verify before acting.',
      })
      renderWithProviders(<RunFailures run={makeRun()} />)

      const toggle = await screen.findByRole('button', { name: /toggle ai failure summary/i })
      await user.click(toggle)
      expect(await screen.findByText('AI hypothesis')).toBeInTheDocument()

      await user.click(toggle)
      await waitFor(() => {
        expect(screen.queryByText('AI hypothesis')).not.toBeInTheDocument()
      })
    })
  })
})
