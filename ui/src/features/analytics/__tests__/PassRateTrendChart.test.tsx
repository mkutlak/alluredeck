import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { PassRateTrendChart } from '../PassRateTrendChart'
import type { PassRateTrendPoint } from '@/lib/chart-utils'

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

const sampleData: PassRateTrendPoint[] = [
  { name: '#1', passRate: 85 },
  { name: '#2', passRate: 92 },
  { name: '#3', passRate: 78 },
]

describe('PassRateTrendChart', () => {
  it('uses ChartContainer (renders data-chart attribute)', () => {
    render(<PassRateTrendChart data={sampleData} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })

  it('renders with empty data without crashing', () => {
    render(<PassRateTrendChart data={[]} />)
  })

  it('renders with single data point without crashing', () => {
    render(<PassRateTrendChart data={[{ name: '#1', passRate: 100 }]} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })
})
