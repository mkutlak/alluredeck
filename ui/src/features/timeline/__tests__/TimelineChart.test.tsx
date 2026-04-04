import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineTestCase } from '@/types/api'
import { TimelineChart } from '../TimelineChart'

// ---------------------------------------------------------------------------
// Mock all sub-components
// ---------------------------------------------------------------------------

vi.mock('../TimelineMinimap', () => ({
  TimelineMinimap: (props: Record<string, unknown>) => (
    <div data-testid="mock-minimap" data-props={JSON.stringify(props)} />
  ),
}))
vi.mock('../TimelineGanttChart', () => ({
  TimelineGanttChart: (props: Record<string, unknown>) => (
    <div data-testid="mock-gantt" data-props={JSON.stringify(props)} />
  ),
}))
vi.mock('../GanttTooltip', () => ({
  GanttTooltip: () => null,
}))
vi.mock('../TimelineLegend', () => ({
  TimelineLegend: (props: Record<string, unknown>) => (
    <div data-testid="mock-legend" data-props={JSON.stringify(props)} />
  ),
}))
vi.mock('../TimelineDetailTable', () => ({
  TimelineDetailTable: (props: Record<string, unknown>) => (
    <div data-testid="mock-detail-table" data-props={JSON.stringify(props)} />
  ),
}))
vi.mock('@/hooks/useStatusColors', () => ({
  useStatusColors: () => ({
    passed: '#40a02b',
    failed: '#d20f39',
    broken: '#fe640b',
    skipped: '#8c8fa1',
  }),
}))
vi.mock('@/hooks/useContainerWidth', () => ({
  useContainerWidth: () => 1000,
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeTestCase(overrides: Partial<TimelineTestCase> = {}): TimelineTestCase {
  return {
    name: 'Login test',
    full_name: 'com.example.LoginTest',
    status: 'passed',
    start: 1000000,
    stop: 1005000,
    duration: 5000,
    thread: 'main',
    host: 'host-1',
    ...overrides,
  }
}

const MIN_START = 1000000
const MAX_STOP = 1020000

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('TimelineChart (orchestrator)', () => {
  it('renders the minimap', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('mock-minimap')).toBeInTheDocument()
  })

  it('renders the gantt chart', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('mock-gantt')).toBeInTheDocument()
  })

  it('renders the legend', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('mock-legend')).toBeInTheDocument()
  })

  it('renders the detail table', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)
    expect(screen.getByTestId('mock-detail-table')).toBeInTheDocument()
  })

  it('passes testCases to sub-components', () => {
    const testCases = [
      makeTestCase({ name: 'alpha' }),
      makeTestCase({ name: 'beta', start: 1010000, stop: 1015000 }),
    ]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)

    const legendProps = JSON.parse(
      screen.getByTestId('mock-legend').getAttribute('data-props') ?? '{}',
    )
    expect(legendProps.testCases).toHaveLength(2)

    const ganttProps = JSON.parse(
      screen.getByTestId('mock-gantt').getAttribute('data-props') ?? '{}',
    )
    expect(ganttProps.testCases).toHaveLength(2)
  })

  it('passes statusColors to sub-components', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={MIN_START} maxStop={MAX_STOP} />)

    const legendProps = JSON.parse(
      screen.getByTestId('mock-legend').getAttribute('data-props') ?? '{}',
    )
    expect(legendProps.statusColors).toEqual({
      passed: '#40a02b',
      failed: '#d20f39',
      broken: '#fe640b',
      skipped: '#8c8fa1',
    })
  })

  it('renders with empty testCases without crashing', () => {
    render(<TimelineChart testCases={[]} minStart={MIN_START} maxStop={MAX_STOP} />)
    // Legend and detail table always render (HTML, not SVG)
    expect(screen.getByTestId('mock-legend')).toBeInTheDocument()
    expect(screen.getByTestId('mock-detail-table')).toBeInTheDocument()
  })
})
