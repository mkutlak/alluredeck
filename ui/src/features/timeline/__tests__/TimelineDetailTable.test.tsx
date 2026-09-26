import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, within, act, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { TimelineTestCase } from '@/types/api'
import { TimelineDetailTable } from '../TimelineDetailTable'

const makeTC = (name: string, fullName: string, duration: number): TimelineTestCase => ({
  name,
  full_name: fullName,
  status: 'passed',
  start: 0,
  stop: duration,
  duration,
  thread: 'worker-1',
  host: 'node-a',
})

// Alphabetical order (Average, Fast, Slow) differs from both duration orders.
const testCases = [
  makeTC('Fast test', 'suite.fast', 100),
  makeTC('Slow test', 'suite.slow', 30000),
  makeTC('Average test', 'suite.average', 5000),
]

const colors = { passed: '#40a02b', failed: '#d20f39', broken: '#fe640b', skipped: '#8c8fa1' }

function renderTable(cases = testCases, onTestClick = vi.fn()) {
  render(<TimelineDetailTable testCases={cases} statusColors={colors} onTestClick={onTestClick} />)
  return onTestClick
}

const rowNames = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => within(row).getAllByRole('cell')[0]!.textContent)

describe('TimelineDetailTable', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('sorts slowest first, toggles duration and sorts by name from the headers', async () => {
    const user = userEvent.setup()
    renderTable()
    expect(rowNames()).toEqual(['Slow test', 'Average test', 'Fast test'])

    const duration = screen.getByRole('columnheader', { name: /duration/i })
    await user.click(duration)
    expect(rowNames()).toEqual(['Fast test', 'Average test', 'Slow test'])
    await user.click(duration)
    expect(rowNames()).toEqual(['Slow test', 'Average test', 'Fast test'])

    await user.click(screen.getByRole('columnheader', { name: /name/i }))
    expect(rowNames()).toEqual(['Average test', 'Fast test', 'Slow test'])
  })

  it('filters rows by the debounced search and says when nothing matches', async () => {
    vi.useFakeTimers()
    renderTable()
    const search = async (value: string) => {
      fireEvent.change(screen.getByPlaceholderText(/search tests/i), { target: { value } })
      await act(async () => {
        vi.advanceTimersByTime(300)
      })
    }

    await search('slow')
    expect(rowNames()).toEqual(['Slow test'])

    await search('nonexistent-xyz')
    expect(screen.getByText(/no tests match/i)).toBeInTheDocument()
  })

  it('row click calls onTestClick with the correct test case', async () => {
    const user = userEvent.setup()
    const onTestClick = renderTable()

    // Second data row under the default slowest-first sort.
    await user.click(screen.getByRole('row', { name: /average test/i }))

    expect(onTestClick).toHaveBeenCalledOnce()
    expect(onTestClick).toHaveBeenCalledWith(testCases[2])
  })

  it('handles empty testCases array without crashing', () => {
    renderTable([])
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.queryByText(/no tests match/i)).not.toBeInTheDocument()
  })
})
