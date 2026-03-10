import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CategoryBreakdownChart } from '../CategoryBreakdownChart'
import type { CategoryBreakdownPoint } from '@/lib/chart-utils'

// Recharts uses ResizeObserver + SVG layout; stub it out in jsdom
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div style={{ width: 500, height: 200 }}>{children}</div>
    ),
  }
})

const sampleData: CategoryBreakdownPoint[] = [
  { name: 'Product defects', failed: 3, broken: 1, total: 4, color: '#d20f39' },
  { name: 'Test defects', failed: 1, broken: 2, total: 3, color: '#fe640b' },
]

describe('CategoryBreakdownChart', () => {
  it('renders "No defect categories" when data is empty', () => {
    render(<CategoryBreakdownChart data={[]} />)
    expect(screen.getByText(/No defect categories/i)).toBeInTheDocument()
  })

  it('does not render empty state when data is provided', () => {
    render(<CategoryBreakdownChart data={sampleData} />)
    expect(screen.queryByText(/No defect categories/i)).not.toBeInTheDocument()
  })

  it('renders category names in the chart when data is provided', () => {
    render(<CategoryBreakdownChart data={sampleData} />)
    expect(screen.getByText('Product defects')).toBeInTheDocument()
    expect(screen.getByText('Test defects')).toBeInTheDocument()
  })
})
