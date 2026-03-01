import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { PassRateSparkline } from '../PassRateSparkline'
import type { DashboardSparklinePoint } from '@/types/api'

// Stub ResponsiveContainer (used internally by ChartContainer) to work in jsdom
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div style={{ width: 200, height: 48 }}>{children}</div>
    ),
  }
})

const sampleData: DashboardSparklinePoint[] = [
  { build_order: 1, pass_rate: 80 },
  { build_order: 2, pass_rate: 85 },
  { build_order: 3, pass_rate: 92 },
]

describe('PassRateSparkline', () => {
  it('renders without crashing', () => {
    render(<PassRateSparkline data={sampleData} />)
  })

  it('uses ChartContainer (renders data-chart attribute)', () => {
    render(<PassRateSparkline data={sampleData} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })

  it('renders nothing when data is empty', () => {
    const { container } = render(<PassRateSparkline data={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders with single data point without crashing', () => {
    render(<PassRateSparkline data={[{ build_order: 1, pass_rate: 100 }]} />)
  })
})
