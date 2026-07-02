import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CircleCheck } from 'lucide-react'
import { Segmented, type SegmentedOption } from '../segmented'

type View = 'list' | 'grid'

const options: SegmentedOption<View>[] = [
  { value: 'list', label: 'List' },
  { value: 'grid', label: 'Grid', count: 3, 'data-testid': 'grid-option' },
]

describe('Segmented', () => {
  it('renders all options', () => {
    render(
      <Segmented value="list" onValueChange={vi.fn()} options={options} aria-label="View mode" />,
    )
    expect(screen.getByRole('button', { name: /^List$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Grid\(3\)/ })).toBeInTheDocument()
  })

  it('sets aria-pressed only on the active option', () => {
    render(
      <Segmented value="list" onValueChange={vi.fn()} options={options} aria-label="View mode" />,
    )
    expect(screen.getByRole('button', { name: /^List$/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: /Grid\(3\)/ })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  it('fires onValueChange with the clicked value', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(
      <Segmented
        value="list"
        onValueChange={onValueChange}
        options={options}
        aria-label="View mode"
      />,
    )
    await user.click(screen.getByRole('button', { name: /Grid\(3\)/ }))
    expect(onValueChange).toHaveBeenCalledWith('grid')
  })

  it('renders count as a muted "(n)" suffix', () => {
    render(
      <Segmented value="list" onValueChange={vi.fn()} options={options} aria-label="View mode" />,
    )
    expect(screen.getByText('(3)', { exact: false })).toBeInTheDocument()
  })

  it('passes through data-testid per option', () => {
    render(
      <Segmented value="list" onValueChange={vi.fn()} options={options} aria-label="View mode" />,
    )
    expect(screen.getByTestId('grid-option')).toBeInTheDocument()
  })

  it('applies the group role and aria-label to the container', () => {
    render(
      <Segmented value="list" onValueChange={vi.fn()} options={options} aria-label="View mode" />,
    )
    expect(screen.getByRole('group', { name: 'View mode' })).toBeInTheDocument()
  })

  it('renders a leading icon when provided', () => {
    const iconOptions: SegmentedOption<View>[] = [
      { value: 'list', label: 'List', icon: CircleCheck },
      { value: 'grid', label: 'Grid' },
    ]
    render(
      <Segmented
        value="list"
        onValueChange={vi.fn()}
        options={iconOptions}
        aria-label="View mode"
      />,
    )
    const button = screen.getByRole('button', { name: /^List$/ })
    expect(button.querySelector('svg')).toBeInTheDocument()
  })
})
