import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { StatusTrendChart } from '../StatusTrendChart'
import type { StatusTrendPoint } from '@/lib/chart-utils'

// Stub ResponsiveContainer (used internally by ChartContainer) to work in jsdom
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div style={{ width: 500, height: 300 }}>{children}</div>
    ),
  }
})

const sampleData: StatusTrendPoint[] = [
  { name: '#1', passed: 10, failed: 2, broken: 1, skipped: 0 },
  { name: '#2', passed: 12, failed: 1, broken: 0, skipped: 1 },
]

describe('StatusTrendChart', () => {
  it('renders without crashing', () => {
    render(<StatusTrendChart data={sampleData} />)
  })

  it('uses ChartContainer (renders data-chart attribute)', () => {
    render(<StatusTrendChart data={sampleData} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })

  it('renders with empty data without crashing', () => {
    render(<StatusTrendChart data={[]} />)
  })

  it('renders multiple data points without crashing', () => {
    render(<StatusTrendChart data={sampleData} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })
})
