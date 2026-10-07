import { useState } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineRun, PipelineSuite } from '@/types/api'

const fetchRunFailures = vi.fn().mockResolvedValue({
  data: [],
  metadata: { message: 'ok', truncated: false },
})

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: (...args: unknown[]) => fetchRunFailures(...args),
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
    builds: [{ build_id: 105, build_number: 5 }],
    ...overrides,
  }
}

function makeRun(overrides?: Partial<PipelineRun>): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234def5678',
    branch: 'main',
    ci_build_url: 'https://ci.example.com/pipelines/123',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites: [
      makeSuite(),
      makeSuite({
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
      }),
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

function allPassingRun(): PipelineRun {
  return makeRun({
    suites: [makeSuite()],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
    },
  })
}

beforeEach(() => {
  fetchRunFailures.mockClear()
})

describe('RunRow', () => {
  it.each([
    { run: makeRun(), id: '196765' },
    // No pipeline id: fall back to the truncated SHA.
    { run: makeRun({ pipeline_id: undefined }), id: 'abc1234' },
  ])('identifies the run by $id, with its branch', ({ run, id }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByText(id)).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('links the group label to the numeric group_project_id, outside the toggle button', async () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    const link = await screen.findByRole('link', { name: /acme/i })
    expect(link).toHaveAttribute('href', '/projects/10')
    expect(screen.getByTestId('run-row-toggle')).not.toContainElement(link)
  })

  // A 95.9% pass rate used to render green on a run that failed, so the loudest
  // colour on the row said "fine". The verdict glyph and the failed-test count
  // carry the colour; the pass rate is a plain fact.
  // The text comes from the counts (127/142, all passed), not from pass_rate.
  it.each([
    {
      name: 'failing',
      run: makeRun({ aggregate: { ...makeRun().aggregate, pass_rate: 95.9 } }),
      text: '89.4%',
    },
    { name: 'passing', run: allPassingRun(), text: '100%' },
  ])('does not colour the pass rate of a $name run', ({ run, text }) => {
    renderWithProviders(<RunRow run={run} />)
    const rate = screen.getByText(text)
    expect(rate.className).not.toMatch(/text-\[#/)
  })

  // The server's pass_rate can overstate; the row rates from counts, floored.
  it('shows 99.9% for 2499 of 2500 passed even when pass_rate says 100', () => {
    const run = makeRun({
      aggregate: { ...makeRun().aggregate, tests_passed: 2499, tests_total: 2500, pass_rate: 100 },
    })
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByText('99.9%')).toBeInTheDocument()
  })

  // Every test skipped: nothing ran, so there is no rate and no verdict. The row
  // must not read as a green pass nor as a red 0%.
  describe('a run where every test was skipped', () => {
    function allSkippedRun(): PipelineRun {
      return makeRun({
        suites: [makeSuite({ total: 5, skipped: 5, passed: 0, pass_rate: 0, status: 'skipped' })],
        aggregate: {
          suites_passed: 0,
          suites_total: 1,
          tests_passed: 0,
          tests_total: 5,
          tests_skipped: 5,
          pass_rate: 0,
          total_duration_ms: 1000,
        },
      })
    }

    it('shows "—" as the rate and "all tests skipped" as the summary', () => {
      renderWithProviders(<RunRow run={allSkippedRun()} />)
      expect(screen.getByText('—')).toBeInTheDocument()
      expect(screen.getByTestId('run-row-summary')).toHaveTextContent('all tests skipped')
      expect(screen.getByTestId('run-row-summary')).not.toHaveTextContent('suites passed')
    })

    it('uses a neutral glyph: no green check, no red anywhere', () => {
      renderWithProviders(<RunRow run={allSkippedRun()} />)
      const row = screen.getByTestId('run-row')
      expect(row).not.toHaveTextContent('✓')
      expect(row).not.toHaveTextContent('✗')
      expect(row.outerHTML).not.toMatch(/d20f39|f38ba8|40a02b|a6e3a1/)
    })
  })

  // A build whose stats could not be read has no counts at all (total 0), yet the
  // API keeps its suite `failed`. Zero counts must not read as "all skipped": that
  // would hide a broken report behind a neutral glyph.
  describe('a run whose suites have no readable stats', () => {
    function noStatsRun(): PipelineRun {
      return makeRun({
        suites: [makeSuite({ total: 0, passed: 0, skipped: 0, pass_rate: 0, status: 'failed' })],
        aggregate: {
          suites_passed: 0,
          suites_total: 1,
          tests_passed: 0,
          tests_total: 0,
          tests_skipped: 0,
          pass_rate: 0,
          total_duration_ms: 1000,
        },
      })
    }

    it('is a failing run: ✗, "—" for the rate, never "all tests skipped"', () => {
      renderWithProviders(<RunRow run={noStatsRun()} />)
      const row = screen.getByTestId('run-row')
      expect(row).toHaveTextContent('✗')
      expect(row).not.toHaveTextContent(/[✓○]/)
      expect(screen.getByText('—')).toBeInTheDocument()
      expect(screen.getByTestId('run-row-summary')).toHaveTextContent('1/1 suites failing')
      expect(screen.getByTestId('run-row-summary')).not.toHaveTextContent('all tests skipped')
    })
  })

  it('makes the failed-test count the one red figure on a failing run', () => {
    renderWithProviders(<RunRow run={makeRun()} />)
    expect(screen.getByText('15 failed tests')).toHaveClass('text-[#d20f39]')
  })

  it('falls back to group_slug when group_project_id is absent', () => {
    renderWithProviders(
      <RunRow run={makeRun({ group_project_id: undefined, group_slug: 'orphan-group' })} />,
    )
    expect(screen.getByText('orphan-group')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /orphan-group/i })).not.toBeInTheDocument()
  })

  // Auto-expanding every failing run is what made a page of ten runs
  // unscannable, and it fetched failures nobody had asked to see.
  it('is collapsed without fetching failures until toggled open, and toggles closed', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RunRow run={makeRun()} />)
    const toggle = screen.getByTestId('run-row-toggle')

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
    expect(fetchRunFailures).not.toHaveBeenCalled()

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(await screen.findByTestId('run-failures')).toBeInTheDocument()
    expect(fetchRunFailures).toHaveBeenCalledTimes(1)

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
  })

  it.each([
    {
      // Failing suites become chips; a green run gets none.
      run: makeRun(),
      summary: '1/2 suites failing · 15 failed tests',
      chips: ['ui-tests'],
    },
    {
      run: allPassingRun(),
      summary: '1/1 suites passed',
      chips: [],
    },
  ])('summarises the run as $summary with chips $chips', ({ run, summary, chips }) => {
    renderWithProviders(<RunRow run={run} />)
    // The failed-test count has its own element (it is the one red figure), so
    // the summary is no longer a single text node.
    expect(screen.getByTestId('run-row-summary')).toHaveTextContent(summary)
    expect(screen.queryAllByTestId('run-suite-chip').map((c) => c.textContent)).toEqual(
      chips.map((c) => expect.stringContaining(c)),
    )
  })

  it.each([
    { failed: 1, text: '1 failed test' },
    { failed: 2, text: '2 failed tests' },
  ])('counts failures in the singular or plural: "$text"', ({ failed, text }) => {
    const run = makeRun({ suites: [makeSuite({ failed, status: 'degraded' })] })
    renderWithProviders(<RunRow run={run} />)
    expect(screen.getByText(text)).toBeInTheDocument()
  })

  // Nothing to expand on a run without failures: the toggle used to open only
  // "No failing tests in this run."
  it.each([
    { name: 'passing', run: allPassingRun() },
    {
      name: 'all-skipped',
      run: makeRun({
        suites: [makeSuite({ total: 5, skipped: 5, passed: 0, status: 'skipped' })],
        aggregate: {
          suites_passed: 0,
          suites_total: 1,
          tests_passed: 0,
          tests_total: 5,
          tests_skipped: 5,
          pass_rate: 0,
          total_duration_ms: 1000,
        },
      }),
    },
  ])('gives a $name run no expand toggle', ({ run }) => {
    renderWithProviders(<RunRow run={run} />)
    expect(screen.queryByTestId('run-row-toggle')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /toggle failures/i })).not.toBeInTheDocument()
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
  })

  // A refetch can leave an expanded run with no failures; its toggle goes away,
  // so the open panel must go with it rather than linger with no way to close.
  it('closes the panel when an expanded run stops having failures', async () => {
    const user = userEvent.setup()
    function Harness() {
      const [run, setRun] = useState(makeRun())
      return (
        <>
          <button onClick={() => setRun(allPassingRun())}>refetched</button>
          <RunRow run={run} />
        </>
      )
    }
    renderWithProviders(<Harness />)

    await user.click(screen.getByTestId('run-row-toggle'))
    expect(await screen.findByTestId('run-failures')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'refetched' }))
    expect(screen.queryByTestId('run-row-toggle')).not.toBeInTheDocument()
    expect(screen.queryByTestId('run-failures')).not.toBeInTheDocument()
  })
})
