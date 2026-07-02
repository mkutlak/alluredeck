import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { FilterBar } from '../FilterBar'

describe('FilterBar', () => {
  it('renders search, filters, and end slots in order', () => {
    render(
      <FilterBar
        search={<span data-testid="search">search</span>}
        filters={<span data-testid="filters">filters</span>}
        end={<span data-testid="end">end</span>}
      />,
    )
    const search = screen.getByTestId('search')
    const filters = screen.getByTestId('filters')
    const end = screen.getByTestId('end')
    expect(search.compareDocumentPosition(filters) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(filters.compareDocumentPosition(end) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('wraps the end slot in a right-aligned container', () => {
    render(<FilterBar end={<span data-testid="end">end</span>} />)
    const end = screen.getByTestId('end')
    expect(end.parentElement?.className).toContain('ml-auto')
  })

  it('renders nothing for absent slots', () => {
    const { container } = render(<FilterBar search={<span>search</span>} />)
    // Only one child (search) should be present; no empty wrapper divs for filters/end
    expect(screen.queryByText('search')).toBeInTheDocument()
    expect(container.querySelectorAll('.ml-auto').length).toBe(0)
  })

  it('renders nothing at all when no slots are given', () => {
    const { container } = render(<FilterBar />)
    expect(container.firstChild?.textContent).toBe('')
  })

  it('applies className passthrough to the root', () => {
    const { container } = render(<FilterBar className="custom-class" />)
    expect(container.firstElementChild?.className).toContain('custom-class')
  })
})
