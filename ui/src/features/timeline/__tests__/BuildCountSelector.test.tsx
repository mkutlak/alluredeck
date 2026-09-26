import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BuildCountSelector } from '../BuildCountSelector'

describe('BuildCountSelector', () => {
  it('shows the value, pluralizes option labels and reports a numeric selection', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<BuildCountSelector value={3} onChange={onChange} />)
    const select = screen.getByRole('combobox', { name: /builds/i })

    expect(select).toHaveValue('3')
    expect(screen.getByRole('option', { name: '1 build' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: '2 builds' })).toBeInTheDocument()

    await user.selectOptions(select, '5')
    expect(onChange).toHaveBeenCalledWith(5)
  })
})
