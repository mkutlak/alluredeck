import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineTestCase, TimelineBuildEntry } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'

// d3 zoom needs real layout; a chainable stub is enough for rendering.
vi.mock('d3-zoom', () => {
  const chain = (): unknown =>
    Object.assign(vi.fn(), {
      scaleExtent: chain,
      translateExtent: chain,
      on: chain,
      filter: chain,
    })
  return { zoom: vi.fn(chain), zoomIdentity: { k: 1, x: 0, y: 0 } }
})
vi.mock('d3-selection', () => ({ select: () => ({ call: vi.fn(), on: vi.fn() }) }))

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

function makeBuild(order: number, testCases: TimelineTestCase[], createdAt: string) {
  return { build_order: order, created_at: createdAt, test_cases: testCases } as TimelineBuildEntry
}

const colors: StatusColorMap = {
  passed: '#40a02b',
  failed: '#d20f39',
  broken: '#fe640b',
  skipped: '#8c8fa1',
}

const baseProps = {
  testCases: [makeTC()],
  minStart: 0,
  maxStop: 10000,
  statusColors: colors,
  width: 800,
  height: 450,
  selectedRange: null,
  onViewportChange: vi.fn(),
  onBrushSelect: vi.fn(),
  highlightedTestId: null,
}

describe('TimelineGanttChart', () => {
  it('each bar has correct fill color based on status', () => {
    const testCases = (['passed', 'failed', 'broken', 'skipped'] as const).map((status, i) =>
      makeTC({ full_name: status, status, start: i * 2000, stop: i * 2000 + 2000 }),
    )
    render(<TimelineGanttChart {...baseProps} testCases={testCases} />)

    expect(screen.getAllByTestId('gantt-bar').map((bar) => bar.getAttribute('fill'))).toEqual([
      '#40a02b',
      '#d20f39',
      '#fe640b',
      '#8c8fa1',
    ])
  })

  it('stacks multiple builds into labelled bands with a separator between them', () => {
    const builds = [
      makeBuild(
        44,
        [makeTC({ full_name: 'a' }), makeTC({ full_name: 'b' })],
        '2026-03-25T00:00:00Z',
      ),
      makeBuild(43, [makeTC({ full_name: 'c' })], '2026-03-24T00:00:00Z'),
    ]
    render(<TimelineGanttChart {...baseProps} builds={builds} />)

    expect(screen.getByText('Build #44 — 2026-03-25')).toBeInTheDocument()
    expect(screen.getByText('Build #43 — 2026-03-24')).toBeInTheDocument()
    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(3)
    expect(screen.getAllByTestId('band-separator')).toHaveLength(1)
  })

  it.each<[string, TimelineBuildEntry[] | undefined]>([
    ['still works in single-build mode (no builds prop)', undefined],
    [
      'still works when builds has single entry',
      [makeBuild(1, [makeTC()], '2026-03-25T00:00:00Z')],
    ],
  ])('%s', (_name, builds) => {
    render(<TimelineGanttChart {...baseProps} builds={builds} />)

    expect(screen.getAllByTestId('gantt-bar')).toHaveLength(1)
    expect(screen.queryByText(/Build #/)).not.toBeInTheDocument()
  })
})
