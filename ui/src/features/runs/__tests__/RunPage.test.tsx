import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { ApiError } from '@/api/client'
import { formatDate } from '@/lib/utils'
import { useUIStore } from '@/store/ui'
import { RunPage } from '../RunPage'
import { RunFailures } from '../RunFailures'
import type { PipelineRun, PipelineSuite, RunFailure, RunFailuresResponse } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRun: vi.fn(),
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', display_name: 'Acme Payments', children: [1, 2, 3] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi
    .fn()
    .mockResolvedValue({ data: { llm_enabled: false }, metadata: { message: 'ok' } }),
}))

import { fetchPipelineRun, fetchRunFailures } from '@/api/pipeline'

const LONG_BRANCH =
  'feature/PAY-99999-multi-region-settlement-reconciliation-with-idempotency-keys-and-retry-budget'

function suite(project_id: number, slug: string, failed: number, extra?: Partial<PipelineSuite>) {
  return {
    project_id,
    slug,
    build_number: 10 + project_id,
    build_id: 100 + project_id,
    pass_rate: 0,
    total: 100,
    passed: 100 - failed,
    failed,
    skipped: 0,
    duration_ms: 42000,
    status: failed > 0 ? 'degraded' : 'passed',
    ...extra,
  } satisfies PipelineSuite
}

const failingRun: PipelineRun = {
  pipeline_id: '48213',
  pipeline_url: 'https://ci.example/48213',
  commit_sha: 'a1b2c3d4e5f6',
  branch: 'feature/sepa',
  timestamp: '2026-10-07T09:12:00Z',
  group_project_id: 10,
  group_slug: 'acme',
  suites: [
    suite(1, 'api-cloud', 3),
    suite(2, 'ui-tests', 4, {
      builds: [1, 2, 3].map((n) => ({ build_id: 200 + n, build_number: n })),
    }),
    suite(3, 'contract-tests', 0),
  ],
  aggregate: {
    suites_passed: 1,
    suites_total: 3,
    tests_passed: 293,
    tests_total: 300,
    tests_skipped: 0,
    pass_rate: 97.6,
    total_duration_ms: 213000,
  },
}

function failure(project_id: number, slug: string, test_name: string): RunFailure {
  return {
    project_id,
    slug,
    build_id: 100 + project_id,
    build_number: 10 + project_id,
    test_name,
    full_name: test_name,
    status: 'failed',
    duration_ms: 100,
    history_id: `h-${test_name}`,
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'AssertionError: boom',
  }
}

const failures = [
  ...['a1', 'a2', 'a3'].map((t) => failure(1, 'api-cloud', t)),
  ...['u1', 'u2', 'u3', 'u4'].map((t) => failure(2, 'ui-tests', t)),
]

function renderRunAt(url: string, run: PipelineRun | Error = failingRun) {
  if (run instanceof Error) vi.mocked(fetchPipelineRun).mockRejectedValue(run)
  else vi.mocked(fetchPipelineRun).mockResolvedValue({ data: run, metadata: { message: 'ok' } })
  const router = createMemoryRouter(
    [
      { path: '/runs/:groupId/:runKey', element: <RunPage /> },
      { path: '/', element: <p>feed</p> },
    ],
    { initialEntries: [url] },
  )
  const view = renderWithProviders(<></>, { router })
  return { router, ...view }
}

const runHeading = () => screen.findByRole('heading', { level: 1 })
const sections = () => screen.findAllByRole('region')

beforeEach(() => {
  vi.mocked(fetchPipelineRun).mockReset()
  vi.mocked(fetchRunFailures).mockReset()
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: failures,
    metadata: { message: 'ok', truncated: false },
  })
  useUIStore.setState({ runsFailureGrouping: 'suite' })
  document.title = 'AllureDeck'
})

describe('RunPage', () => {
  it('fetches the run by group and decoded key from the URL', async () => {
    renderRunAt('/runs/10/gh%2Frun%207')
    await waitFor(() => expect(fetchPipelineRun).toHaveBeenCalledWith('10', 'gh/run 7'))
  })

  it('titles the page with the run ID in mono, the failing glyph beside it', async () => {
    renderRunAt('/runs/10/48213')
    const h1 = await runHeading()
    expect(h1).toHaveAccessibleName('48213')
    expect(within(h1).getByText('48213')).toHaveClass('font-mono')
    expect(h1).toHaveTextContent('✗')
  })

  it('states branch, SHA, group, CI link, time and the summary under the title', async () => {
    renderRunAt('/runs/10/48213')
    await runHeading()

    expect(screen.getByText('feature/sepa')).toBeInTheDocument()
    expect(screen.getByText('a1b2c3d')).toBeInTheDocument()
    expect(await screen.findByRole('link', { name: 'Acme Payments' })).toHaveAttribute(
      'href',
      '/projects/10',
    )
    const ci = screen.getByRole('link', { name: /ci pipeline/i })
    expect(ci).toHaveAttribute('href', 'https://ci.example/48213')
    expect(ci).toHaveAttribute('rel', expect.stringContaining('noopener'))
    expect(screen.getByText(/ago$/)).toHaveAttribute('title', formatDate(failingRun.timestamp))
    expect(screen.getByTestId('run-page-summary')).toHaveTextContent(
      '2/3 suites failing · 7 failed tests · 97.6% passed · 3m 33s',
    )
  })

  // Wrapping, never an ellipsis: the full branch is how the developer knows the run.
  it('shows a long branch whole, without truncation', async () => {
    renderRunAt('/runs/10/48213', { ...failingRun, branch: LONG_BRANCH })
    await runHeading()
    // One element holds the whole name (the feed splits it into head and tail), it is
    // told to wrap, and nothing around it clips, truncates or forbids wrapping.
    const branch = screen.getByText(LONG_BRANCH)
    expect(branch.textContent).toBe(LONG_BRANCH)
    expect(branch).toHaveClass('wrap-anywhere')
    expect(branch.closest('.truncate, .whitespace-nowrap, .overflow-hidden')).toBeNull()
  })

  it('sets the document title on this page only', async () => {
    const { unmount } = renderRunAt('/runs/10/48213')
    await runHeading()
    expect(document.title).toBe('48213 · feature/sepa — AllureDeck')
    unmount()
    expect(document.title).toBe('AllureDeck')
  })

  it('leads with one open section per failing suite, worst first, anchored by project id', async () => {
    renderRunAt('/runs/10/48213')
    const regions = await sections()

    expect(regions.map((r) => r.id)).toEqual(['suite-2', 'suite-1'])
    expect(within(regions[0]!).getByRole('heading', { level: 2 })).toHaveTextContent('ui-tests')
    // Every section is expanded, not only the first.
    expect(within(regions[1]!).getByRole('link', { name: 'a3' })).toBeInTheDocument()
  })

  it("links a failure to the test's history on the run's branch, and its report second", async () => {
    renderRunAt('/runs/10/48213', { ...failingRun, branch: 'feature/a b' })
    expect(await screen.findByRole('link', { name: 'a1' })).toHaveAttribute(
      'href',
      '/projects/1/tests?history_id=h-a1&branch=feature%2Fa+b',
    )
    expect(screen.getByRole('link', { name: 'Allure report for a1' })).toHaveAttribute(
      'href',
      '/projects/1/reports/11',
    )
  })

  it('collapses the passed suites into one quiet line that opens to name, rate and duration', async () => {
    const user = userEvent.setup()
    renderRunAt('/runs/10/48213')
    await sections()

    const suiteLink = screen.getByRole('link', { name: 'contract-tests' })
    expect(suiteLink).not.toBeVisible()
    await user.click(screen.getByText('1 suite passed'))

    expect(suiteLink).toBeVisible()
    expect(suiteLink).toHaveAttribute('href', '/projects/3/reports/13')
    const item = screen.getByRole('listitem')
    expect(item).toHaveTextContent('100%')
    expect(item).toHaveTextContent('42s')
  })

  // A chip's #suite-<id> must land somewhere, even on a collapsed list.
  it('opens the passed list and focuses the suite a hash points at', async () => {
    renderRunAt('/runs/10/48213#suite-3')
    await sections()

    const item = screen.getByRole('listitem')
    expect(item).toBeVisible()
    await waitFor(() => expect(item).toHaveFocus())
  })

  it('reopens and focuses a failing section the user collapsed when a hash points at it', async () => {
    const user = userEvent.setup()
    const { router } = renderRunAt('/runs/10/48213')
    await sections()
    const toggle = screen.getByRole('button', { name: 'api-cloud' })
    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    await act(() => router.navigate('/runs/10/48213#suite-1'))

    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 2, name: 'api-cloud' })).toHaveFocus(),
    )
  })

  // A deep link may show the suite view, but must not rewrite the saved preference.
  it('shows suite sections for a #suite- link while the saved grouping stays "by error"', async () => {
    useUIStore.setState({ runsFailureGrouping: 'error' })
    const { unmount } = renderRunAt('/runs/10/48213#suite-1')

    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 2, name: 'api-cloud' })).toHaveFocus(),
    )
    expect(screen.getByRole('button', { name: 'by suite' })).toHaveAttribute('aria-pressed', 'true')
    expect(useUIStore.getState().runsFailureGrouping).toBe('error')
    unmount()

    // The feed's panel still opens "by error".
    renderWithProviders(<RunFailures run={failingRun} />)
    expect(await screen.findByRole('button', { name: 'by error' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('lets the user switch away from the deep-link view, saving that choice', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ runsFailureGrouping: 'error' })
    renderRunAt('/runs/10/48213#suite-1')
    await sections()

    await user.click(screen.getByRole('button', { name: 'by error' }))

    expect(screen.getByRole('button', { name: 'by error' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.queryAllByRole('region')).toHaveLength(0)
    expect(useUIStore.getState().runsFailureGrouping).toBe('error')
  })

  // The passed list sits below the failures: scrolling to it while they still load
  // would land on a spot the arriving sections then push away.
  it('scrolls to a passed suite only after the failing sections have rendered', async () => {
    let release: (v: RunFailuresResponse) => void = () => {}
    vi.mocked(fetchRunFailures).mockReturnValue(new Promise((r) => (release = r)))
    const regionsAtScroll: number[] = []
    const scroll = vi
      .spyOn(Element.prototype, 'scrollIntoView')
      .mockImplementation(() => regionsAtScroll.push(screen.queryAllByRole('region').length))
    try {
      renderRunAt('/runs/10/48213#suite-3')
      await runHeading()
      expect(screen.getByRole('listitem')).not.toHaveFocus()
      expect(scroll).not.toHaveBeenCalled()

      await act(async () =>
        release({ data: failures, metadata: { message: 'ok', truncated: false } }),
      )

      await waitFor(() => expect(screen.getByRole('listitem')).toHaveFocus())
      expect(regionsAtScroll).toEqual([2])
    } finally {
      scroll.mockRestore()
    }
  })

  it('lands on a failing section once a retried failures request succeeds', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchRunFailures)
      .mockRejectedValueOnce(new Error('network error'))
      .mockResolvedValue({ data: failures, metadata: { message: 'ok', truncated: false } })
    renderRunAt('/runs/10/48213#suite-1')

    await user.click(await screen.findByRole('button', { name: /retry/i }))

    await waitFor(() =>
      expect(screen.getByRole('heading', { level: 2, name: 'api-cloud' })).toHaveFocus(),
    )
  })

  // Rows to group by error do not exist; the sections (and the toggle) must not vanish.
  it('keeps suite sections when grouped by error and no failure rows arrived', async () => {
    useUIStore.setState({ runsFailureGrouping: 'error' })
    vi.mocked(fetchRunFailures).mockResolvedValue({
      data: [],
      metadata: { message: 'ok', truncated: true },
    })
    renderRunAt('/runs/10/48213')

    const regions = await sections()
    expect(regions.map((r) => r.id)).toEqual(['suite-2', 'suite-1'])
    expect(screen.getByRole('button', { name: 'by suite' })).toBeInTheDocument()
    expect(screen.queryByText(/no failing tests in this run/i)).not.toBeInTheDocument()
  })

  // TestHistoryPage rejects an empty history_id, so such a row links its report.
  it('links a failure without a history id to its report', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue({
      data: [{ ...failure(1, 'api-cloud', 'a1'), history_id: '' }],
      metadata: { message: 'ok', truncated: false },
    })
    renderRunAt('/runs/10/48213')
    expect(await screen.findByRole('link', { name: 'a1' })).toHaveAttribute(
      'href',
      '/projects/1/reports/11',
    )
  })

  // Suites with unreadable stats: nothing ran that can be rated, so no "— passed".
  it('leaves the pass rate out of the summary when there is none', async () => {
    renderRunAt('/runs/10/48213', {
      ...failingRun,
      suites: [suite(1, 'api-cloud', 0, { status: 'failed', total: 0, passed: 0 })],
      aggregate: { ...failingRun.aggregate, suites_total: 1, tests_passed: 0, tests_total: 0 },
    })
    await runHeading()
    expect(screen.getByTestId('run-page-summary')).toHaveTextContent('1/1 suites failing · 3m 33s')
    expect(screen.getByTestId('run-page-summary')).not.toHaveTextContent('—')
  })

  // State such as an opened passed list belongs to one run, not to the next one, even
  // when that run is already cached and shows without a loading state in between.
  it('starts fresh when the URL moves to another, cached run', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchPipelineRun).mockImplementation((_group, key) =>
      Promise.resolve({ data: { ...failingRun, pipeline_id: key }, metadata: { message: 'ok' } }),
    )
    const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
      initialEntries: ['/runs/10/48214'],
    })
    renderWithProviders(<></>, { router })
    expect(await screen.findByRole('heading', { level: 1, name: '48214' })).toBeInTheDocument()
    await act(() => router.navigate('/runs/10/48213'))
    expect(await screen.findByRole('heading', { level: 1, name: '48213' })).toBeInTheDocument()
    await user.click(screen.getByText('1 suite passed'))
    expect(screen.getByRole('listitem')).toBeVisible()

    await act(() => router.navigate('/runs/10/48214'))

    expect(screen.getByRole('heading', { level: 1, name: '48214' })).toBeInTheDocument()
    expect(screen.getByRole('listitem')).not.toBeVisible()
  })

  // The API caps failures per run; a suite cut off by that cap still gets its section.
  it('gives a failing suite whose rows were not loaded its section and says why', async () => {
    vi.mocked(fetchRunFailures).mockResolvedValue({
      data: failures.filter((f) => f.project_id === 2),
      metadata: { message: 'ok', truncated: true },
    })
    renderRunAt('/runs/10/48213')

    const regions = await sections()
    expect(regions.map((r) => r.id)).toEqual(['suite-2', 'suite-1'])
    expect(within(regions[1]!).getByText(/not listed/i)).toBeInTheDocument()
  })

  it.each([
    {
      name: 'passing',
      run: {
        ...failingRun,
        suites: [suite(3, 'contract-tests', 0)],
        aggregate: { ...failingRun.aggregate, suites_passed: 1, suites_total: 1 },
      },
      glyph: '✓',
      summary: '1/1 suites passed',
      line: '1 suite passed',
    },
    {
      name: 'all-skipped',
      run: {
        ...failingRun,
        suites: [
          suite(1, 'api-cloud', 0, { status: 'skipped', skipped: 100, passed: 0 }),
          suite(3, 'contract-tests', 0, { status: 'skipped', skipped: 100, passed: 0 }),
        ],
        aggregate: { ...failingRun.aggregate, tests_passed: 0, tests_skipped: 300 },
      },
      glyph: '○',
      summary: 'all tests skipped',
      line: '2 suites skipped',
    },
  ])('shows a $name run as a header and an open suite list, no failures', async (c) => {
    renderRunAt('/runs/10/48213', c.run)
    expect(await runHeading()).toHaveTextContent(c.glyph)
    expect(screen.getByTestId('run-page-summary')).toHaveTextContent(c.summary)
    expect(screen.getByText(c.line)).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')[0]).toBeVisible()
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
    expect(fetchRunFailures).not.toHaveBeenCalled()
  })

  it('shows an in-page "Run not found" with a way back to Runs on a 404', async () => {
    renderRunAt('/runs/10/nope', new ApiError('not found', { status: 404, data: {} }))

    expect(
      await screen.findByRole('heading', { level: 1, name: 'Run not found' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /back to runs/i })).toHaveAttribute('href', '/')
    expect(screen.queryByText(/page not found/i)).not.toBeInTheDocument()
  })

  it('offers Retry on other errors and refetches the run', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchPipelineRun)
      .mockRejectedValueOnce(new ApiError('boom', { status: 500, data: {} }))
      .mockRejectedValueOnce(new ApiError('boom', { status: 500, data: {} }))
      .mockResolvedValue({ data: failingRun, metadata: { message: 'ok' } })
    const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
      initialEntries: ['/runs/10/48213'],
    })
    renderWithProviders(<></>, { router })

    // One automatic retry (about a second) before the error shows.
    await user.click(await screen.findByRole('button', { name: /retry/i }, { timeout: 4000 }))
    expect(await runHeading()).toHaveAccessibleName('48213')
  })

  // TanStack resets a query without data to pending on refetch, so the error block (and
  // its Retry) gives way to the skeleton: nothing to click twice.
  it('takes Retry away while the run is being fetched again', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchPipelineRun)
      .mockRejectedValueOnce(new ApiError('boom', { status: 500, data: {} }))
      .mockRejectedValueOnce(new ApiError('boom', { status: 500, data: {} }))
      .mockReturnValue(new Promise(() => {}))
    const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
      initialEntries: ['/runs/10/48213'],
    })
    renderWithProviders(<></>, { router })

    const retry = await screen.findByRole('button', { name: /retry/i }, { timeout: 4000 })
    expect(retry).toBeEnabled()
    await user.click(retry)
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument(),
    )
    expect(screen.queryByRole('heading', { level: 1 })).not.toBeInTheDocument()
  })
})
