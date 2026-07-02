import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SearchInput } from '../SearchInput'

describe('SearchInput', () => {
  it('renders a leading search icon', () => {
    const { container } = render(
      <SearchInput value="" onChange={vi.fn()} aria-label="Search tests" />,
    )
    expect(container.querySelector('svg')).toBeInTheDocument()
  })

  it('applies the required aria-label', () => {
    render(<SearchInput value="" onChange={vi.fn()} aria-label="Search tests" />)
    expect(screen.getByRole('textbox', { name: 'Search tests' })).toBeInTheDocument()
  })

  it('fires onChange when typing', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<SearchInput value="" onChange={onChange} aria-label="Search tests" />)
    await user.type(screen.getByRole('textbox', { name: 'Search tests' }), 'abc')
    expect(onChange).toHaveBeenCalled()
  })

  it('reflects the controlled value', () => {
    render(<SearchInput value="hello" onChange={vi.fn()} aria-label="Search tests" />)
    expect(screen.getByRole('textbox', { name: 'Search tests' })).toHaveValue('hello')
  })

  it('applies placeholder when provided', () => {
    render(
      <SearchInput value="" onChange={vi.fn()} aria-label="Search tests" placeholder="Search…" />,
    )
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument()
  })

  it('merges className overrides', () => {
    render(<SearchInput value="" onChange={vi.fn()} aria-label="Search tests" className="w-32" />)
    expect(screen.getByRole('textbox', { name: 'Search tests' }).className).toContain('w-32')
  })
})
