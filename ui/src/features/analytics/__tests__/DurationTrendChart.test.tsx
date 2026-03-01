import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { DurationTrendChart } from '../DurationTrendChart'
import type { DurationTrendPoint } from '@/lib/chart-utils'

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

const sampleData: DurationTrendPoint[] = [
  { name: '#1', durationSec: 42 },
  { name: '#2', durationSec: 67 },
  { name: '#3', durationSec: 35 },
]

describe('DurationTrendChart', () => {
  it('renders without crashing', () => {
    render(<DurationTrendChart data={sampleData} />)
  })

  it('uses ChartContainer (renders data-chart attribute)', () => {
    render(<DurationTrendChart data={sampleData} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })

  it('renders with empty data without crashing', () => {
    render(<DurationTrendChart data={[]} />)
  })

  it('renders with single data point without crashing', () => {
    render(<DurationTrendChart data={[{ name: '#1', durationSec: 120 }]} />)
    expect(document.querySelector('[data-chart]')).not.toBeNull()
  })
})
