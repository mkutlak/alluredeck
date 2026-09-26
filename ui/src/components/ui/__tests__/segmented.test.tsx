import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Segmented, type SegmentedOption } from '../segmented'

type View = 'list' | 'grid'

const options: SegmentedOption<View>[] = [
  { value: 'list', label: 'List' },
  { value: 'grid', label: 'Grid', count: 3 },
]

describe('Segmented', () => {
  it('presses only the active option, shows counts, and reports clicks by value', async () => {
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

    expect(screen.getByRole('button', { name: /^List$/ })).toHaveAttribute('aria-pressed', 'true')
    const grid = screen.getByRole('button', { name: /Grid\(3\)/ })
    expect(grid).toHaveAttribute('aria-pressed', 'false')

    await user.click(grid)
    expect(onValueChange).toHaveBeenCalledWith('grid')
  })
})
