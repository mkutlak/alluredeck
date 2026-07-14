import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DefectFilters, type DefectFilterValues } from '../DefectFilters'

function renderFilters(overrides: Partial<DefectFilterValues> = {}, onFilterChange = vi.fn()) {
  const filters: DefectFilterValues = {
    category: '',
    resolution: '',
    sort: 'last_seen',
    search: '',
    ...overrides,
  }
  render(<DefectFilters filters={filters} onFilterChange={onFilterChange} />)
  return { onFilterChange }
}

describe('DefectFilters', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders a search input with the current value', () => {
    renderFilters({ search: 'null pointer' })
    expect(screen.getByLabelText('Search defects')).toHaveValue('null pointer')
  })

  it('calls onFilterChange with updated search text', async () => {
    const user = userEvent.setup()
    const { onFilterChange } = renderFilters()
    await user.type(screen.getByLabelText('Search defects'), 'x')
    expect(onFilterChange).toHaveBeenCalledWith(expect.objectContaining({ search: 'x' }))
  })

  it('renders category, resolution, and sort as comboboxes defaulting to "All"', () => {
    renderFilters()
    const comboboxes = screen.getAllByRole('combobox')
    expect(comboboxes).toHaveLength(3)
    expect(comboboxes[0]).toHaveTextContent(/all categories/i)
    expect(comboboxes[1]).toHaveTextContent(/all resolutions/i)
    expect(comboboxes[2]).toHaveTextContent(/last seen/i)
  })

  it('selecting a category maps the UI value back to the API contract', async () => {
    const user = userEvent.setup()
    const { onFilterChange } = renderFilters()

    const comboboxes = screen.getAllByRole('combobox')
    await user.click(comboboxes[0]!)
    const option = await screen.findByRole('option', { name: /^product bug$/i })
    await user.click(option)

    expect(onFilterChange).toHaveBeenCalledWith(
      expect.objectContaining({ category: 'product_bug' }),
    )
  })

  it('selecting "All categories" maps back to the empty-string sentinel', async () => {
    const user = userEvent.setup()
    const { onFilterChange } = renderFilters({ category: 'product_bug' })

    const comboboxes = screen.getAllByRole('combobox')
    await user.click(comboboxes[0]!)
    const option = await screen.findByRole('option', { name: /^all categories$/i })
    await user.click(option)

    expect(onFilterChange).toHaveBeenCalledWith(expect.objectContaining({ category: '' }))
  })

  it('selecting a resolution maps the UI value back to the API contract', async () => {
    const user = userEvent.setup()
    const { onFilterChange } = renderFilters()

    const comboboxes = screen.getAllByRole('combobox')
    await user.click(comboboxes[1]!)
    const option = await screen.findByRole('option', { name: /^fixed$/i })
    await user.click(option)

    expect(onFilterChange).toHaveBeenCalledWith(expect.objectContaining({ resolution: 'fixed' }))
  })

  it('selecting "All resolutions" maps back to the empty-string sentinel', async () => {
    const user = userEvent.setup()
    const { onFilterChange } = renderFilters({ resolution: 'fixed' })

    const comboboxes = screen.getAllByRole('combobox')
    await user.click(comboboxes[1]!)
    const option = await screen.findByRole('option', { name: /^all resolutions$/i })
    await user.click(option)

    expect(onFilterChange).toHaveBeenCalledWith(expect.objectContaining({ resolution: '' }))
  })

  it('selecting a sort option propagates the raw value', async () => {
    const user = userEvent.setup()
    const { onFilterChange } = renderFilters()

    const comboboxes = screen.getAllByRole('combobox')
    await user.click(comboboxes[2]!)
    const option = await screen.findByRole('option', { name: /^occurrences$/i })
    await user.click(option)

    expect(onFilterChange).toHaveBeenCalledWith(
      expect.objectContaining({ sort: 'occurrence_count' }),
    )
  })
})
