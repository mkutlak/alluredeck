import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { StatusPieChart } from '../StatusPieChart'
import type { StatusPiePoint } from '@/lib/chart-utils'

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

const sampleData: StatusPiePoint[] = [
  { name: 'passed', value: 10, color: 'var(--chart-1)' },
  { name: 'failed', value: 2, color: 'var(--chart-2)' },
  { name: 'broken', value: 1, color: 'var(--chart-3)' },
]

describe('StatusPieChart', () => {
  it('renders without crashing', () => {
    render(<StatusPieChart data={sampleData} total={13} />)
  })

  it('uses ChartContainer (renders data-chart attribute)', () => {
    render(<StatusPieChart data={sampleData} total={13} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })

  it('renders with empty data without crashing', () => {
    render(<StatusPieChart data={[]} total={0} />)
  })

  it('displays the total count', () => {
    const { container } = render(<StatusPieChart data={sampleData} total={13} />)
    expect(container.textContent).toContain('13')
  })
})
