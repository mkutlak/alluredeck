import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Combobox, type ComboboxOption } from './combobox'

const OPTIONS: ComboboxOption[] = [
  { value: 'apple', label: 'Apple' },
  { value: 'banana', label: 'Banana' },
  { value: 'cherry', label: 'Cherry' },
]

function renderCombobox(value: string | null, allowClear = false) {
  const onChange = vi.fn()
  const view = render(
    <Combobox
      options={OPTIONS}
      value={value}
      onChange={onChange}
      allowClear={allowClear}
      placeholder="Pick a fruit"
    />,
  )
  return { ...view, onChange }
}

describe('Combobox', () => {
  it.each([
    [null, 'Pick a fruit'],
    ['banana', 'Banana'],
  ])('value %j shows %s on the trigger', (value, text) => {
    renderCombobox(value)
    expect(screen.getByRole('combobox')).toHaveTextContent(text)
  })

  it('calls onChange with the selected value when user picks an item', async () => {
    const user = userEvent.setup()
    const { onChange } = renderCombobox(null)

    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByText('Cherry'))

    expect(onChange).toHaveBeenCalledExactlyOnceWith('cherry')
  })

  it('offers Clear selection only with allowClear, and clearing calls onChange(null)', async () => {
    const user = userEvent.setup()
    const { unmount } = renderCombobox('apple')
    await user.click(screen.getByRole('combobox'))
    expect(screen.queryByText('Clear selection')).not.toBeInTheDocument()
    unmount()

    const { onChange } = renderCombobox('apple', true)
    await user.click(screen.getByRole('combobox'))
    await user.click(screen.getByText('Clear selection'))
    expect(onChange).toHaveBeenCalledExactlyOnceWith(null)
  })
})
