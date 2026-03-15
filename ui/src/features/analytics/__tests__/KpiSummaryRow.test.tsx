import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Mock recharts to avoid SVG rendering issues in jsdom
vi.mock('recharts', () => ({
  LineChart: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Line: () => null,
}))

// Import AFTER mocks
import { KpiSummaryRow } from '../KpiSummaryRow'
import type { KpiData } from '@/lib/chart-utils'

const mockData: KpiData = {
  passRate: 92,
  passRateTrend: [88, 90, 91, 92],
  totalTests: 417,
  totalTestsTrend: [400, 410, 415, 417],
  avgDuration: 2400,
  durationTrend: [3000, 2800, 2500, 2400],
  failedCount: 3,
  failedTrend: [5, 4, 3, 3],
}

describe('KpiSummaryRow', () => {
  it('renders all 4 KPI values', () => {
    render(<KpiSummaryRow data={mockData} />)
    expect(screen.getByText('92%')).toBeInTheDocument()
    expect(screen.getByText('417')).toBeInTheDocument()
    expect(screen.getByText('2s')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('renders all 4 KPI labels', () => {
    render(<KpiSummaryRow data={mockData} />)
    expect(screen.getByText('Pass Rate')).toBeInTheDocument()
    expect(screen.getByText('Total Tests')).toBeInTheDocument()
    expect(screen.getByText('Avg Duration')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
  })
})
