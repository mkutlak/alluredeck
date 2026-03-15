import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineTestCase } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'

// Mock D3 modules since zoom/brush don't work in jsdom
vi.mock('d3-zoom', () => {
  const identity = { k: 1, x: 0, y: 0 }
  return {
    zoom: vi.fn(() => {
      const z = Object.assign(vi.fn(() => z), {
        scaleExtent: vi.fn(() => z),
        translateExtent: vi.fn(() => z),
        on: vi.fn(() => z),
        filter: vi.fn(() => z),
      })
      return z
    }),
    zoomIdentity: identity,
  }
})

vi.mock('d3-selection', () => ({
  select: vi.fn(() => {
    const sel = {
      call: vi.fn(() => sel),
      on: vi.fn(() => sel),
    }
    return sel
  }),
}))

import { TimelineGanttChart } from '../TimelineGanttChart'

const makeTC = (overrides: Partial<TimelineTestCase> = {}): TimelineTestCase => ({
  name: 'test',
  full_name: 'suite.test',
  status: 'passed',
  start: 0,
  stop: 1000,
  duration: 1000,
  thread: '',
  host: '',
  ...overrides,
})

const defaultColors: StatusColorMap = {
  passed: '#40a02b',
  failed: '#d20f39',
  broken: '#fe640b',
  skipped: '#8c8fa1',
}

const noop = vi.fn()

const defaultProps = {
  testCases: [makeTC()],
  minStart: 0,
  maxStop: 10000,
  statusColors: defaultColors,
  width: 800,
  height: 450,
  selectedRange: null as [number, number] | null,
  onViewportChange: noop,
  onBrushSelect: noop,
  highlightedTestId: null as string | null,
}

describe('TimelineGanttChart', () => {
  it('renders an SVG element with data-testid="gantt-chart"', () => {
    render(<TimelineGanttChart {...defaultProps} />)
    expect(screen.getByTestId('gantt-chart')).toBeInTheDocument()
  })

  it('SVG has role="img" and aria-label for accessibility', () => {
    render(<TimelineGanttChart {...defaultProps} />)
    const svg = screen.getByTestId('gantt-chart')
    expect(svg).toHaveAttribute('role', 'img')
    expect(svg).toHaveAttribute('aria-label', 'Test execution timeline')
  })

  it('renders correct number of bar rects (data-testid="gantt-bar")', () => {
    const testCases = [
      makeTC({ name: 'a', full_name: 'a', start: 0, stop: 3000 }),
      makeTC({ name: 'b', full_name: 'b', start: 3000, stop: 6000 }),
    ]
    render(<TimelineGanttChart {...defaultProps} testCases={testCases} />)
    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(2)
  })

  it('each bar has correct fill color based on status', () => {
    const testCases = [
      makeTC({ name: 'p', full_name: 'p', status: 'passed', start: 0, stop: 2000 }),
      makeTC({ name: 'f', full_name: 'f', status: 'failed', start: 2000, stop: 4000 }),
      makeTC({ name: 'b', full_name: 'b', status: 'broken', start: 4000, stop: 6000 }),
      makeTC({ name: 's', full_name: 's', status: 'skipped', start: 6000, stop: 8000 }),
    ]
    render(<TimelineGanttChart {...defaultProps} testCases={testCases} />)
    const bars = screen.getAllByTestId('gantt-bar')
    expect(bars[0]).toHaveAttribute('fill', '#40a02b')
    expect(bars[1]).toHaveAttribute('fill', '#d20f39')
    expect(bars[2]).toHaveAttribute('fill', '#fe640b')
    expect(bars[3]).toHaveAttribute('fill', '#8c8fa1')
  })

  it('renders time axis group (data-testid="time-axis")', () => {
    render(<TimelineGanttChart {...defaultProps} />)
    expect(screen.getByTestId('time-axis')).toBeInTheDocument()
  })

  it('has tabIndex=0 for keyboard focus', () => {
    render(<TimelineGanttChart {...defaultProps} />)
    const svg = screen.getByTestId('gantt-chart')
    expect(svg).toHaveAttribute('tabindex', '0')
  })

  it('has a clipPath element', () => {
    render(<TimelineGanttChart {...defaultProps} />)
    const svg = screen.getByTestId('gantt-chart')
    const clipPath = svg.querySelector('clipPath')
    expect(clipPath).not.toBeNull()
  })

  it('renders with empty testCases without crashing (0 bars)', () => {
    render(<TimelineGanttChart {...defaultProps} testCases={[]} />)
    expect(screen.getByTestId('gantt-chart')).toBeInTheDocument()
    expect(screen.queryAllByTestId('gantt-bar')).toHaveLength(0)
  })

  it('shows correct bar count with mixed statuses', () => {
    const testCases = [
      makeTC({ name: 'a', full_name: 'a', status: 'passed', start: 0, stop: 2000 }),
      makeTC({ name: 'b', full_name: 'b', status: 'failed', start: 2000, stop: 5000 }),
      makeTC({ name: 'c', full_name: 'c', status: 'broken', start: 5000, stop: 8000 }),
    ]
    render(<TimelineGanttChart {...defaultProps} testCases={testCases} />)
    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(3)
  })
})
