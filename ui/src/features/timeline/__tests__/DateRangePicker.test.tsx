import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DateRangePicker } from '../DateRangePicker'

describe('DateRangePicker', () => {
  const onRangeChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders two date inputs', () => {
    render(<DateRangePicker from={undefined} to={undefined} onRangeChange={onRangeChange} />)
    expect(screen.getByLabelText(/from/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/to/i)).toBeInTheDocument()
  })

  it('renders with provided from and to values', () => {
    render(<DateRangePicker from="2026-01-01" to="2026-03-01" onRangeChange={onRangeChange} />)
    const fromInput = screen.getByLabelText(/from/i) as HTMLInputElement
    const toInput = screen.getByLabelText(/to/i) as HTMLInputElement
    expect(fromInput.value).toBe('2026-01-01')
    expect(toInput.value).toBe('2026-03-01')
  })

  it('calls onRangeChange when from date changes', async () => {
    const user = userEvent.setup()
    render(<DateRangePicker from={undefined} to="2026-03-01" onRangeChange={onRangeChange} />)
    const fromInput = screen.getByLabelText(/from/i)
    await user.clear(fromInput)
    await user.type(fromInput, '2026-01-15')
    expect(onRangeChange).toHaveBeenCalledWith('2026-01-15', '2026-03-01')
  })

  it('calls onRangeChange when to date changes', async () => {
    const user = userEvent.setup()
    render(<DateRangePicker from="2026-01-01" to={undefined} onRangeChange={onRangeChange} />)
    const toInput = screen.getByLabelText(/to/i)
    await user.clear(toInput)
    await user.type(toInput, '2026-03-25')
    expect(onRangeChange).toHaveBeenCalledWith('2026-01-01', '2026-03-25')
  })

  it('shows clear button when a date range is set', () => {
    render(<DateRangePicker from="2026-01-01" to="2026-03-01" onRangeChange={onRangeChange} />)
    expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument()
  })

  it('does not show clear button when no dates are set', () => {
    render(<DateRangePicker from={undefined} to={undefined} onRangeChange={onRangeChange} />)
    expect(screen.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument()
  })

  it('calls onRangeChange with undefined values when clear is clicked', async () => {
    const user = userEvent.setup()
    render(<DateRangePicker from="2026-01-01" to="2026-03-01" onRangeChange={onRangeChange} />)
    await user.click(screen.getByRole('button', { name: /clear/i }))
    expect(onRangeChange).toHaveBeenCalledWith(undefined, undefined)
  })
})
