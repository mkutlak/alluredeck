import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineTestCase, TimelineBuildEntry } from '@/types/api'
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

function makeBuild(
  buildOrder: number,
  testCases: TimelineTestCase[],
  createdAt = '2026-03-25T12:00:00Z',
): TimelineBuildEntry {
  return {
    build_order: buildOrder,
    created_at: createdAt,
    test_cases: testCases,
    summary: {
      total: testCases.length,
      min_start: Math.min(...testCases.map((tc) => tc.start)),
      max_stop: Math.max(...testCases.map((tc) => tc.stop)),
      total_duration: testCases.reduce((sum, tc) => sum + tc.duration, 0),
      truncated: false,
    },
  }
}

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

describe('TimelineGanttChart (multi-build)', () => {
  it('renders band labels when multiple builds are provided', () => {
    const builds = [
      makeBuild(44, [makeTC({ start: 0, stop: 3000 })], '2026-03-25T00:00:00Z'),
      makeBuild(43, [makeTC({ start: 0, stop: 2000 })], '2026-03-24T00:00:00Z'),
    ]
    render(<TimelineGanttChart {...defaultProps} builds={builds} />)

    expect(screen.getByText(/Build #44/)).toBeInTheDocument()
    expect(screen.getByText(/Build #43/)).toBeInTheDocument()
  })

  it('renders bars from all builds', () => {
    const builds = [
      makeBuild(2, [
        makeTC({ name: 'a', full_name: 'a', start: 0, stop: 3000 }),
        makeTC({ name: 'b', full_name: 'b', start: 3000, stop: 6000 }),
      ]),
      makeBuild(1, [
        makeTC({ name: 'c', full_name: 'c', start: 0, stop: 2000 }),
      ]),
    ]
    render(<TimelineGanttChart {...defaultProps} builds={builds} />)
    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(3)
  })

  it('still works in single-build mode (no builds prop)', () => {
    render(<TimelineGanttChart {...defaultProps} />)
    expect(screen.getByTestId('gantt-chart')).toBeInTheDocument()
    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(1)
    // No band labels in single-build mode
    expect(screen.queryByText(/Build #/)).not.toBeInTheDocument()
  })

  it('still works when builds has single entry', () => {
    const builds = [makeBuild(1, [makeTC()])]
    render(<TimelineGanttChart {...defaultProps} builds={builds} />)
    expect(screen.getByTestId('gantt-chart')).toBeInTheDocument()
    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(1)
    // Single build should not show band labels
    expect(screen.queryByText(/Build #/)).not.toBeInTheDocument()
  })

  it('renders horizontal separator lines between bands', () => {
    const builds = [
      makeBuild(2, [makeTC({ start: 0, stop: 3000 })]),
      makeBuild(1, [makeTC({ start: 0, stop: 2000 })]),
    ]
    render(<TimelineGanttChart {...defaultProps} builds={builds} />)
    const svg = screen.getByTestId('gantt-chart')
    // Should have separator lines (data-testid="band-separator")
    const separators = svg.querySelectorAll('[data-testid="band-separator"]')
    expect(separators.length).toBeGreaterThanOrEqual(1)
  })
})
