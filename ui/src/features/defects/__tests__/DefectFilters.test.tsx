import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DefectFilters, type DefectFilterValues } from '../DefectFilters'

function renderFilters(overrides: Partial<DefectFilterValues> = {}) {
  const onFilterChange = vi.fn()
  const filters: DefectFilterValues = {
    category: '',
    resolution: '',
    sort: 'last_seen',
    search: '',
    ...overrides,
  }
  render(<DefectFilters filters={filters} onFilterChange={onFilterChange} />)
  return onFilterChange
}

describe('DefectFilters', () => {
  it('calls onFilterChange with updated search text', async () => {
    const onFilterChange = renderFilters()
    await userEvent.type(screen.getByLabelText('Search defects'), 'x')
    expect(onFilterChange).toHaveBeenCalledWith(expect.objectContaining({ search: 'x' }))
  })

  // Radix Select forbids '' as an item value; the UI shows an "all" sentinel instead.
  it('shows the empty-string filters as "All" and the default sort', () => {
    renderFilters()
    expect(screen.getByRole('combobox', { name: /category/i })).toHaveTextContent(/all categories/i)
    expect(screen.getByRole('combobox', { name: /resolution/i })).toHaveTextContent(
      /all resolutions/i,
    )
    expect(screen.getByRole('combobox', { name: /sort by/i })).toHaveTextContent(/last seen/i)
  })

  it.each([
    { select: /category/i, from: {}, option: /^product bug$/i, want: { category: 'product_bug' } },
    {
      select: /category/i,
      from: { category: 'product_bug' as const },
      option: /^all categories$/i,
      want: { category: '' },
    },
    { select: /resolution/i, from: {}, option: /^fixed$/i, want: { resolution: 'fixed' } },
    {
      select: /resolution/i,
      from: { resolution: 'fixed' as const },
      option: /^all resolutions$/i,
      want: { resolution: '' },
    },
    { select: /sort by/i, from: {}, option: /^occurrences$/i, want: { sort: 'occurrence_count' } },
  ])('maps option $option back to $want', async ({ select, from, option, want }) => {
    const user = userEvent.setup()
    const onFilterChange = renderFilters(from)

    await user.click(screen.getByRole('combobox', { name: select }))
    await user.click(await screen.findByRole('option', { name: option }))

    expect(onFilterChange).toHaveBeenCalledWith(expect.objectContaining(want))
  })
})
