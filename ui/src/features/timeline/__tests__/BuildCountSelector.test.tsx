import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BuildCountSelector } from '../BuildCountSelector'

describe('BuildCountSelector', () => {
  const onChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a select element with the correct value', () => {
    render(<BuildCountSelector value={3} onChange={onChange} />)
    const select = screen.getByRole('combobox', { name: /builds/i })
    expect(select).toBeInTheDocument()
    expect(select).toHaveValue('3')
  })

  it('has options from 1 to 10', () => {
    render(<BuildCountSelector value={1} onChange={onChange} />)
    const options = screen.getAllByRole('option')
    expect(options).toHaveLength(10)
    expect(options[0]).toHaveValue('1')
    expect(options[9]).toHaveValue('10')
  })

  it('calls onChange with numeric value when selection changes', async () => {
    const user = userEvent.setup()
    render(<BuildCountSelector value={1} onChange={onChange} />)
    const select = screen.getByRole('combobox', { name: /builds/i })
    await user.selectOptions(select, '5')
    expect(onChange).toHaveBeenCalledWith(5)
  })

  it('displays option labels with "build" / "builds" text', () => {
    render(<BuildCountSelector value={1} onChange={onChange} />)
    const options = screen.getAllByRole('option')
    expect(options[0]).toHaveTextContent('1 build')
    expect(options[1]).toHaveTextContent('2 builds')
  })
})
