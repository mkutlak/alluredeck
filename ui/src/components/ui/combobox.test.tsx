import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Combobox, type ComboboxOption } from './combobox'

const OPTIONS: ComboboxOption[] = [
  { value: 'apple', label: 'Apple' },
  { value: 'banana', label: 'Banana' },
  { value: 'cherry', label: 'Cherry' },
]

describe('Combobox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows placeholder when value is null', () => {
    render(<Combobox options={OPTIONS} value={null} onChange={vi.fn()} placeholder="Pick a fruit" />)
    const trigger = screen.getByRole('combobox')
    expect(trigger).toBeInTheDocument()
    expect(trigger).toHaveTextContent('Pick a fruit')
  })

  it('shows matched label when value is set', () => {
    render(<Combobox options={OPTIONS} value="banana" onChange={vi.fn()} placeholder="Pick a fruit" />)
    const trigger = screen.getByRole('combobox')
    expect(trigger).toBeInTheDocument()
    expect(trigger).toHaveTextContent('Banana')
  })

  it('calls onChange with the selected value when user picks an item', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Combobox options={OPTIONS} value={null} onChange={onChange} placeholder="Pick a fruit" />)

    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByText('Cherry'))

    expect(onChange).toHaveBeenCalledOnce()
    expect(onChange).toHaveBeenCalledWith('cherry')
  })

  it('calls onChange(null) when user clicks Clear selection', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <Combobox
        options={OPTIONS}
        value="apple"
        onChange={onChange}
        allowClear
        placeholder="Pick a fruit"
      />,
    )

    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByText('Clear selection'))

    expect(onChange).toHaveBeenCalledOnce()
    expect(onChange).toHaveBeenCalledWith(null)
  })

  it('does not render Clear selection when allowClear is false', async () => {
    const user = userEvent.setup()
    render(<Combobox options={OPTIONS} value="apple" onChange={vi.fn()} placeholder="Pick a fruit" />)

    await user.click(screen.getByRole('combobox'))
    expect(screen.queryByText('Clear selection')).not.toBeInTheDocument()
  })
})
