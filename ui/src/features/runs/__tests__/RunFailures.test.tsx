import { screen, waitFor, within } from '@testing-library/react'
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

function renderRun(failures: RunFailure[], run = makeRun(), truncated = false) {
  const response: RunFailuresResponse = { data: failures, metadata: { message: 'ok', truncated } }
  vi.mocked(fetchRunFailures).mockResolvedValue(response)
  return renderWithProviders(<RunFailures run={run} />)
}

const summaryToggle = { name: /toggle ai failure summary/i }

beforeEach(() => {
  vi.mocked(fetchRunFailures).mockReset()
  vi.mocked(fetchFailureSummary).mockReset()
  vi.mocked(getConfig).mockReset()
  vi.mocked(getConfig).mockResolvedValue(makeConfig({ llm_enabled: false }))
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

describe('RunFailures', () => {
  // Links use the numeric project id and build number of the failure's suite.
  it.each([
    { name: 'CI pipeline id', run: makeRun(), key: '196765' },
    {
      name: 'commit SHA (no pipeline id)',
      run: makeRun({ pipeline_id: undefined }),
      key: 'abc1234',
    },
  ])('fetches the whole run in one request keyed by $name', async ({ run, key }) => {
    renderRun([makeFailure()], run)

    const link = await screen.findByRole('link', { name: 'should login' })
    expect(link).toHaveAttribute('href', '/projects/2/reports/3')
    expect(fetchRunFailures).toHaveBeenCalledTimes(1)
    expect(fetchRunFailures).toHaveBeenCalledWith(10, key)
  })

  it.each([
    {
      name: 'a failed request',
      setup: () => vi.mocked(fetchRunFailures).mockRejectedValue(new Error('network error')),
      run: makeRun(),
      text: /failed to load failures/i,
    },
    {
      name: 'no failing tests',
      setup: () =>
        vi.mocked(fetchRunFailures).mockResolvedValue({
          data: [],
          metadata: { message: 'ok', truncated: false },
        }),
      run: makeRun(),
      text: /no failing tests in this run/i,
    },
    {
      name: 'unavailable without a group project',
      setup: () => {},
      run: makeRun({ group_project_id: undefined }),
      text: /failure details are unavailable/i,
    },
  ])('reports $name', async ({ setup, run, text }) => {
    setup()
    renderWithProviders(<RunFailures run={run} />)

    expect(await screen.findByText(text)).toBeInTheDocument()
    if (run.group_project_id == null) expect(fetchRunFailures).not.toHaveBeenCalled()
  })

  it('notes when the API truncated the result', async () => {
    renderRun([makeFailure()], makeRun(), true)
    expect(await screen.findByText(/this run has more/i)).toBeInTheDocument()
  })

  // The same test arrives twice with different history_ids because two
  // ingestion paths write it; only one copy carries the error message.
  it('collapses duplicate copies of one test into a single row with merged badges', async () => {
    renderRun([
      makeFailure({ history_id: '1ab6c50a.d93c', retries: 3, flaky: true, error_message: '' }),
      makeFailure({ history_id: '462170f6:d93c', new_failed: true, known: true }),
    ])

    await waitFor(() => {
      expect(screen.getAllByTestId('run-failure-row')).toHaveLength(1)
    })
    expect(screen.getByText('1 failure · 1 suite · 1 new')).toBeInTheDocument()
    expect(screen.getByTestId('flaky-badge')).toHaveTextContent('flaky · 3x')
    expect(screen.getByText('new')).toBeInTheDocument()
    expect(screen.getByText('known')).toBeInTheDocument()
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

    it('groups by suite by default, opening only the first group until one is clicked', async () => {
      const user = userEvent.setup()
      renderRun(twoSuites)

      await waitFor(() => {
        expect(screen.getAllByTestId('run-failure-group')).toHaveLength(2)
      })
      expect(screen.getByText('ui-tests')).toBeInTheDocument()
      // ui-tests has two failures and sorts first; its rows show, api-tests' do not.
      expect(screen.getByText('a')).toBeInTheDocument()
      expect(screen.queryByText('c')).not.toBeInTheDocument()

      await user.click(screen.getByText('api-tests'))
      expect(screen.getByText('c')).toBeInTheDocument()
    })

    // 18+ failures per run routinely share one message differing only in a
    // timeout value; by-error states that once.
    it('clusters failures by normalised error signature and persists the choice', async () => {
      const user = userEvent.setup()
      renderRun([
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
      ])

      await user.click(await screen.findByTestId('run-failure-grouping-error'))

      await waitFor(() => {
        expect(screen.getAllByTestId('run-failure-group')).toHaveLength(2)
      })
      // The group is titled with the first real message verbatim; the
      // normalised signature only decides the grouping.
      expect(screen.getByText('Timed out 5000ms waiting for expect(locator)')).toBeInTheDocument()
      expect(screen.getByText('×2 · ui-tests')).toBeInTheDocument()
      expect(useUIStore.getState().runsFailureGrouping).toBe('error')
    })
  })

  // The group header used to repeat the per-suite count the chip above it
  // already shows ("N failed"), and by-error repeats it in "×N".
  it.each(['suite', 'error'] as const)(
    'does not print a "N failed" count in the group headers when grouped by %s',
    async (grouping) => {
      useUIStore.setState({ runsFailureGrouping: grouping })
      renderRun([
        makeFailure({ test_name: 'a', full_name: 'a.js:1:1' }),
        makeFailure({ test_name: 'b', full_name: 'b.js:1:1' }),
      ])

      await waitFor(() => {
        expect(screen.getAllByTestId('run-failure-group')).toHaveLength(1)
      })
      expect(screen.queryByText(/^\d+ failed$/)).not.toBeInTheDocument()
    },
  )

  describe('error text', () => {
    const long =
      'AssertionError: expected 200 to equal 503 at https://sandbox.example.com/api/v1/payments'

    // The message stays plain, selectable text (a button would swallow a drag to
    // copy); a small separate toggle reveals the rest, reachable by keyboard.
    it('clamps a long error to two lines and expands it from a separate toggle', async () => {
      const user = userEvent.setup()
      renderRun([makeFailure({ error_message: long })])

      const text = await screen.findByText(long)
      expect(text.closest('button')).toBeNull()
      expect(text).toHaveClass('line-clamp-2')

      const toggle = screen.getByRole('button', { name: 'Show full error' })
      expect(toggle).toHaveAttribute('aria-expanded', 'false')
      expect(toggle).toHaveAttribute('aria-controls', text.id)

      toggle.focus()
      await user.keyboard('{Enter}')
      expect(toggle).toHaveAccessibleName('Hide full error')
      expect(toggle).toHaveAttribute('aria-expanded', 'true')
      expect(text).not.toHaveClass('line-clamp-2')

      await user.keyboard('{Enter}')
      expect(toggle).toHaveAccessibleName('Show full error')
      expect(text).toHaveClass('line-clamp-2')
    })

    it.each([
      {
        name: 'a short single-line message',
        message: 'TimeoutError: locator.click',
        toggle: false,
      },
      { name: 'a message over 80 characters', message: long, toggle: true },
      { name: 'a multi-line message', message: 'Error: boom\n    at foo.js:1:1', toggle: true },
      { name: 'no message', message: '', toggle: false },
    ])(
      'offers "Show full error" only when it can hide something: $name',
      async ({ message, toggle }) => {
        renderRun([makeFailure({ error_message: message })])
        const row = await screen.findByTestId('run-failure-row')
        expect(!!within(row).queryByRole('button', { name: 'Show full error' })).toBe(toggle)
      },
    )
  })

  // A multi-line Playwright error made one title 4097px wide and pushed the
  // feed 4291px wide. The title is the message's first line only.
  it('titles an error group with the first line of the message, not the whole stack', async () => {
    const user = userEvent.setup()
    const stack =
      'Error: expected locator to be visible\n\n    at login.spec.js:10:5\n    at run (runner.js:2:2)'
    renderRun([makeFailure({ error_message: stack })])
    await user.click(await screen.findByTestId('run-failure-grouping-error'))

    const title = await screen.findByText('Error: expected locator to be visible')
    expect(title).toHaveClass('truncate', 'min-w-0')
    expect(title).toHaveAttribute('title', 'Error: expected locator to be visible')
    const toggle = screen.getByRole('button', { name: /expected locator to be visible/ })
    expect(toggle).not.toHaveAccessibleName(expect.stringContaining('login.spec.js'))
  })

  it('offers Retry on a failed load and refetches the failures', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchRunFailures).mockRejectedValueOnce(new Error('network error'))
    vi.mocked(fetchRunFailures).mockResolvedValue({
      data: [makeFailure()],
      metadata: { message: 'ok', truncated: false },
    })
    renderWithProviders(<RunFailures run={makeRun()} />)

    expect(await screen.findByText(/failed to load failures/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /retry/i }))

    expect(await screen.findByRole('link', { name: 'should login' })).toBeInTheDocument()
    expect(fetchRunFailures).toHaveBeenCalledTimes(2)
  })

  describe('sharded suites', () => {
    it('links every contributing build from the group header', async () => {
      const builds = [1, 2, 3].map((n) => ({ build_id: 100 + n, build_number: n }))
      renderRun([makeFailure()], makeRun({ suites: [makeSuite({ builds })] }))

      expect(await screen.findByText('3 shards')).toBeInTheDocument()
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
    it.each([
      {
        name: 'llm_enabled is false',
        config: () => Promise.resolve(makeConfig({ llm_enabled: false })),
      },
      { name: 'config is still resolving', config: () => new Promise<never>(() => {}) },
    ])('does not render the toggle when $name', async ({ config }) => {
      vi.mocked(getConfig).mockReturnValue(config())
      renderRun([makeFailure()])

      expect(await screen.findByText('should login')).toBeInTheDocument()
      expect(screen.queryByRole('button', summaryToggle)).not.toBeInTheDocument()
    })

    it('toggles a collapsed summary panel for the failure build and history id', async () => {
      const user = userEvent.setup()
      vi.mocked(getConfig).mockResolvedValue(makeConfig())
      vi.mocked(fetchFailureSummary).mockResolvedValue({
        enabled: true,
        summary: { hypothesis: 'Looks like a stale selector.', category: 'test_bug', evidence: [] },
        disclaimer: 'AI hypothesis — verify before acting.',
      })
      renderRun([makeFailure()])

      const toggle = await screen.findByRole('button', summaryToggle)
      expect(toggle).toHaveAttribute('aria-expanded', 'false')

      await user.click(toggle)
      expect(await screen.findByText('AI hypothesis')).toBeInTheDocument()
      expect(fetchFailureSummary).toHaveBeenCalledWith(2, 103, 'h1')

      await user.click(toggle)
      await waitFor(() => {
        expect(screen.queryByText('AI hypothesis')).not.toBeInTheDocument()
      })
    })
  })
})
