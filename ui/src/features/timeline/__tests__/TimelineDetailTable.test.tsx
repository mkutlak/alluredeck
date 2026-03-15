import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, within, act, fireEvent } from '@testing-library/react'
// act and fireEvent used in search/debounce tests below
import userEvent from '@testing-library/user-event'
import type { TimelineTestCase } from '@/types/api'
import { TimelineDetailTable } from '../TimelineDetailTable'

const makeTC = (overrides: Partial<TimelineTestCase> = {}): TimelineTestCase => ({
  name: 'Login test',
  full_name: 'auth.LoginTest.login',
  status: 'passed',
  start: 1000,
  stop: 6000,
  duration: 5000,
  thread: 'worker-1',
  host: 'node-a',
  ...overrides,
})

const defaultColors = {
  passed: '#40a02b',
  failed: '#d20f39',
  broken: '#fe640b',
  skipped: '#8c8fa1',
}

const testCases = [
  makeTC({ name: 'Fast test', full_name: 'suite.fast', duration: 100, start: 0, stop: 100 }),
  makeTC({
    name: 'Slow test',
    full_name: 'suite.slow',
    duration: 30000,
    start: 0,
    stop: 30000,
    status: 'failed',
  }),
  makeTC({ name: 'Medium test', full_name: 'suite.medium', duration: 5000, start: 100, stop: 5100 }),
]

describe('TimelineDetailTable', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('has data-testid="timeline-detail-table"', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    expect(screen.getByTestId('timeline-detail-table')).toBeInTheDocument()
  })

  it('renders table with column headers: Name, Status, Duration, Worker', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    expect(screen.getByRole('columnheader', { name: /name/i })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /status/i })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /duration/i })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /worker/i })).toBeInTheDocument()
  })

  it('renders one row per test case', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    const rows = screen.getAllByRole('row')
    // rows include header row
    expect(rows).toHaveLength(testCases.length + 1)
  })

  it('shows test name in each row', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    expect(screen.getByText('Fast test')).toBeInTheDocument()
    expect(screen.getByText('Slow test')).toBeInTheDocument()
    expect(screen.getByText('Medium test')).toBeInTheDocument()
  })

  it('shows status badge in each row', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    expect(screen.getAllByText('passed').length).toBeGreaterThan(0)
    expect(screen.getAllByText('failed').length).toBeGreaterThan(0)
  })

  it('shows formatted duration in each row', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    // 30000ms = 30s
    expect(screen.getByText('30s')).toBeInTheDocument()
    // 5000ms = 5s
    expect(screen.getByText('5s')).toBeInTheDocument()
    // 100ms = 0s
    expect(screen.getByText('0s')).toBeInTheDocument()
  })

  it('shows worker/thread in each row', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    // All test cases use worker-1 thread
    const workerCells = screen.getAllByText('worker-1')
    expect(workerCells).toHaveLength(testCases.length)
  })

  it('default sort is by duration descending (slowest first)', () => {
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    const rows = screen.getAllByRole('row').slice(1) // skip header
    // Slowest first: Slow test (30000ms), Medium test (5000ms), Fast test (100ms)
    expect(within(rows[0]).getByText('Slow test')).toBeInTheDocument()
    expect(within(rows[1]).getByText('Medium test')).toBeInTheDocument()
    expect(within(rows[2]).getByText('Fast test')).toBeInTheDocument()
  })

  it('clicking Duration header toggles sort direction', async () => {
    const user = userEvent.setup()
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    const durationHeader = screen.getByRole('columnheader', { name: /duration/i })

    // Default is desc (slowest first), click to switch to asc (fastest first)
    await user.click(durationHeader)
    const rowsAsc = screen.getAllByRole('row').slice(1)
    expect(within(rowsAsc[0]).getByText('Fast test')).toBeInTheDocument()
    expect(within(rowsAsc[1]).getByText('Medium test')).toBeInTheDocument()
    expect(within(rowsAsc[2]).getByText('Slow test')).toBeInTheDocument()

    // Click again to go back to desc (slowest first)
    await user.click(durationHeader)
    const rowsDesc = screen.getAllByRole('row').slice(1)
    expect(within(rowsDesc[0]).getByText('Slow test')).toBeInTheDocument()
  })

  it('clicking Name header sorts alphabetically', async () => {
    const user = userEvent.setup()
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )
    const nameHeader = screen.getByRole('columnheader', { name: /name/i })
    await user.click(nameHeader)

    const rows = screen.getAllByRole('row').slice(1)
    // Alphabetical: Fast test, Medium test, Slow test
    expect(within(rows[0]).getByText('Fast test')).toBeInTheDocument()
    expect(within(rows[1]).getByText('Medium test')).toBeInTheDocument()
    expect(within(rows[2]).getByText('Slow test')).toBeInTheDocument()
  })

  it('search input filters rows by name', async () => {
    vi.useFakeTimers()

    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )

    const searchInput = screen.getByPlaceholderText(/search tests/i)
    fireEvent.change(searchInput, { target: { value: 'slow' } })
    await act(async () => {
      vi.advanceTimersByTime(300)
    })

    expect(screen.getByText('Slow test')).toBeInTheDocument()
    expect(screen.queryByText('Fast test')).toBeNull()
    expect(screen.queryByText('Medium test')).toBeNull()
  })

  it('shows "No tests match" message when search yields no results', async () => {
    vi.useFakeTimers()

    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={vi.fn()}
      />,
    )

    const searchInput = screen.getByPlaceholderText(/search tests/i)
    fireEvent.change(searchInput, { target: { value: 'nonexistent-xyz' } })
    await act(async () => {
      vi.advanceTimersByTime(300)
    })

    expect(screen.getByText(/no tests match/i)).toBeInTheDocument()
  })

  it('row click calls onTestClick with the correct test case', async () => {
    const user = userEvent.setup()
    const onTestClick = vi.fn()
    render(
      <TimelineDetailTable
        testCases={testCases}
        statusColors={defaultColors}
        onTestClick={onTestClick}
      />,
    )

    // Default sort: slowest first, so first row is Slow test
    const rows = screen.getAllByRole('row').slice(1)
    await user.click(rows[0])

    expect(onTestClick).toHaveBeenCalledOnce()
    expect(onTestClick).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Slow test', full_name: 'suite.slow' }),
    )
  })

  it('handles empty testCases array without crashing', () => {
    render(
      <TimelineDetailTable testCases={[]} statusColors={defaultColors} onTestClick={vi.fn()} />,
    )
    expect(screen.getByTestId('timeline-detail-table')).toBeInTheDocument()
    // No rows (not even the "no tests match" message since search is empty)
    expect(screen.queryByRole('row')).toBeNull()
  })
})
