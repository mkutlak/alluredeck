import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { mockRecharts } from '@/test/mocks/recharts'
import { CategoryBreakdownChart } from '../CategoryBreakdownChart'

mockRecharts()

describe('CategoryBreakdownChart', () => {
  it('lists each category with its total', () => {
    render(
      <CategoryBreakdownChart
        data={[
          { name: 'Product defects', failed: 3, broken: 1, total: 4, color: '#d20f39' },
          { name: 'Test defects', failed: 1, broken: 2, total: 3, color: '#fe640b' },
        ]}
      />,
    )
    expect(screen.getByRole('list')).toHaveTextContent(/^Product defects4Test defects3$/)
    expect(screen.queryByText(/no defect categories/i)).not.toBeInTheDocument()
  })

  it('renders "No defect categories" when data is empty', () => {
    render(<CategoryBreakdownChart data={[]} />)
    expect(screen.getByText(/no defect categories/i)).toBeInTheDocument()
  })
})
