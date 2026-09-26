import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DateRangePicker } from '../DateRangePicker'

describe('DateRangePicker', () => {
  // Changing one end reports the new value together with the other, untouched end.
  // [title, label, from, to, typed, reported range]
  it.each<[string, RegExp, string | undefined, string | undefined, string, string[]]>([
    [
      'calls onRangeChange when from date changes',
      /^from$/i,
      undefined,
      '2026-03-01',
      '2026-01-15',
      ['2026-01-15', '2026-03-01'],
    ],
    [
      'calls onRangeChange when to date changes',
      /^to$/i,
      '2026-01-01',
      undefined,
      '2026-03-25',
      ['2026-01-01', '2026-03-25'],
    ],
  ])('%s', async (_name, label, from, to, typed, want) => {
    const user = userEvent.setup()
    const onRangeChange = vi.fn()
    render(<DateRangePicker from={from} to={to} onRangeChange={onRangeChange} />)

    await user.type(screen.getByLabelText(label), typed)
    expect(onRangeChange).toHaveBeenCalledWith(...want)
  })

  it('shows the range and clears both ends', async () => {
    const user = userEvent.setup()
    const onRangeChange = vi.fn()
    render(<DateRangePicker from="2026-01-01" to="2026-03-01" onRangeChange={onRangeChange} />)
    expect(screen.getByLabelText(/from/i)).toHaveValue('2026-01-01')
    expect(screen.getByLabelText(/to/i)).toHaveValue('2026-03-01')

    await user.click(screen.getByRole('button', { name: /clear/i }))
    expect(onRangeChange).toHaveBeenCalledWith(undefined, undefined)
  })

  it('does not show clear button when no dates are set', () => {
    render(<DateRangePicker from={undefined} to={undefined} onRangeChange={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument()
  })
})
