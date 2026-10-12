import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type { ApiResponse, ConfigData, PipelineRun, RunFailure } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi.fn(),
}))

import { fetchRunFailures } from '@/api/pipeline'
import { getConfig } from '@/api/system'

const failure = (testName: string, overrides?: Partial<RunFailure>): RunFailure => ({
  project_id: 2,
  slug: 'ui-tests',
  build_id: 103,
  build_number: 3,
  test_name: testName,
  full_name: `${testName}.spec.js:1:1`,
  status: 'failed',
  duration_ms: 100,
  history_id: `h-${testName}`,
  flaky: false,
  retries: 0,
  new_failed: false,
  known: false,
  error_message: `Error in ${testName}`,
  ...overrides,
})

const run: PipelineRun = {
  pipeline_id: '196765',
  commit_sha: 'abc1234',
  branch: 'main',
  timestamp: '2026-04-03T18:00:00Z',
  group_project_id: 10,
  group_slug: 'acme',
  suites: [
    {
      project_id: 2,
      slug: 'ui-tests',
      build_number: 3,
      build_id: 103,
      pass_rate: 98,
      total: 100,
      failed: 2,
      duration_ms: 30000,
      status: 'degraded',
      builds: [{ build_id: 103, build_number: 3 }],
    },
  ],
  aggregate: {
    suites_passed: 0,
    suites_total: 1,
    tests_passed: 98,
    tests_total: 100,
    pass_rate: 98,
    total_duration_ms: 30000,
  },
}

function renderWith(llmEnabled: boolean, errorMessage?: string) {
  vi.mocked(getConfig).mockResolvedValue({
    data: { llm_enabled: llmEnabled } as ConfigData,
    metadata: { message: 'ok' },
  } satisfies ApiResponse<ConfigData>)
  vi.mocked(fetchRunFailures).mockResolvedValue({
    data: [
      // Different badge sets: before, each row sized its own badge column.
      failure('plain', { origin: 'new', ...(errorMessage && { error_message: errorMessage }) }),
      failure('noisy', {
        origin: 'inherited',
        flaky: true,
        retries: 2,
        known: true,
        ...(errorMessage && { error_message: errorMessage }),
      }),
    ],
    metadata: { message: 'ok', truncated: false },
  })
  renderWithProviders(<RunFailures run={run} />)
}

/** Tracks in a grid's @xl template, e.g. "minmax(0,1fr)_10rem_auto" is 3. */
const xlTracks = (el: HTMLElement) =>
  /@xl:grid-cols-\[([^\]]+)\]/.exec(el.className)?.[1]?.split('_').length

/** The grid line holding a failure row's cells: the name link's parent. */
const cellsOf = async (testName: string) =>
  (await screen.findByRole('link', { name: testName })).parentElement as HTMLElement

beforeEach(() => {
  useUIStore.setState({ runsFailureGrouping: 'suite' })
})

describe('failure rows on one locked grid per group', () => {
  // The Locked-Column Rule: the error text started at a different x on every row.
  it('lays every row of a group on the group grid, so its columns start at one x', async () => {
    renderWith(true)
    await screen.findByRole('button', { name: 'Toggle AI failure summary for plain' })
    const lines = [await cellsOf('plain'), await cellsOf('noisy')]
    const rows = screen.getAllByTestId('run-failure-row')
    const group = rows[0]?.parentElement as HTMLElement

    for (const row of rows) {
      expect(row.parentElement).toBe(group)
      expect(row).toHaveClass('col-span-full', 'grid-cols-subgrid')
    }
    for (const line of lines) expect(line).toHaveClass('col-span-full', 'grid-cols-subgrid')
    // name · badges · error · AI
    expect(xlTracks(group)).toBe(4)
    expect(lines[0]?.children).toHaveLength(4)
  })

  it('locks the columns in "by error" mode too', async () => {
    useUIStore.setState({ runsFailureGrouping: 'error' })
    renderWith(true, 'Error: the same message')
    await screen.findByRole('button', { name: 'Toggle AI failure summary for plain' })
    const lines = [await cellsOf('plain'), await cellsOf('noisy')]
    const group = lines[0]?.parentElement?.parentElement as HTMLElement

    expect(lines[1]?.parentElement?.parentElement).toBe(group)
    for (const line of lines) expect(line).toHaveClass('col-span-full', 'grid-cols-subgrid')
    // name · suite · badges · AI
    expect(xlTracks(group)).toBe(4)
    expect(lines[0]?.children).toHaveLength(4)
  })

  // Narrow, the AI control comes after the error on screen as in the DOM: packing it
  // beside the badges sent keyboard focus from "Show full error" back up to it.
  it('keeps the AI control after the error when the panel is narrow', async () => {
    renderWith(true)
    const line = await cellsOf('plain')

    expect(line).not.toHaveClass('grid-flow-dense')
    const badgesCell = [...line.children].find((cell) => cell.contains(screen.getByText('new')))
    expect(badgesCell).toHaveClass('col-span-2')
  })

  it('puts the AI control last, after the error text, as "AI summary"', async () => {
    renderWith(true)
    const line = await cellsOf('plain')
    const toggle = screen.getByRole('button', { name: 'Toggle AI failure summary for plain' })

    expect(toggle).toHaveTextContent(/^AI summary$/)
    expect(line.lastElementChild).toBe(toggle)
    expect(line).toContainElement(screen.getByText('Error in plain'))
    expect(
      screen.getByText('Error in plain').compareDocumentPosition(toggle) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it('gives the AI control no column when the feature is off', async () => {
    renderWith(false)
    const line = await cellsOf('plain')
    const group = screen.getAllByTestId('run-failure-row')[0]?.parentElement as HTMLElement

    expect(line.lastElementChild).toContainElement(screen.getByText('Error in plain'))
    // name · badges · error
    expect(xlTracks(group)).toBe(3)
    expect(line.children).toHaveLength(3)
  })
})
