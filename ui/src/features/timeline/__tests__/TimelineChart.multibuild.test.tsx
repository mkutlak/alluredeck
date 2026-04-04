import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineTestCase, TimelineBuildEntry, MultiTimelineData } from '@/types/api'
import { TimelineChart } from '../TimelineChart'

// ---------------------------------------------------------------------------
// Mock all sub-components
// ---------------------------------------------------------------------------

vi.mock('../TimelineMinimap', () => ({
  TimelineMinimap: (props: Record<string, unknown>) => (
    <div data-testid="mock-minimap" data-tc-count={(props.testCases as unknown[]).length} />
  ),
}))
vi.mock('../TimelineGanttChart', () => ({
  TimelineGanttChart: (props: Record<string, unknown>) => (
    <div
      data-testid="mock-gantt"
      data-has-builds={props.builds !== undefined ? 'true' : 'false'}
      data-builds-count={Array.isArray(props.builds) ? (props.builds as unknown[]).length : 0}
      data-tc-count={(props.testCases as unknown[]).length}
    />
  ),
}))
vi.mock('../GanttTooltip', () => ({
  GanttTooltip: () => null,
}))
vi.mock('../TimelineLegend', () => ({
  TimelineLegend: (props: Record<string, unknown>) => (
    <div data-testid="mock-legend" data-tc-count={(props.testCases as unknown[]).length} />
  ),
}))
vi.mock('../TimelineDetailTable', () => ({
  TimelineDetailTable: (props: Record<string, unknown>) => (
    <div data-testid="mock-detail-table" data-tc-count={(props.testCases as unknown[]).length} />
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('TimelineChart (multi-build data flow)', () => {
  it('accepts MultiTimelineData and renders sub-components', () => {
    const tc1 = makeTestCase({ name: 'a' })
    const tc2 = makeTestCase({ name: 'b', start: 1010000, stop: 1015000 })
    const multiData: MultiTimelineData = {
      builds: [makeBuild(2, [tc1]), makeBuild(1, [tc2])],
      total_builds_in_range: 2,
      builds_returned: 2,
      global_min_start: 1000000,
      global_max_stop: 1015000,
    }
    render(
      <TimelineChart
        builds={multiData.builds}
        testCases={[tc1, tc2]}
        minStart={multiData.global_min_start}
        maxStop={multiData.global_max_stop}
      />,
    )

    expect(screen.getByTestId('mock-minimap')).toBeInTheDocument()
    expect(screen.getByTestId('mock-gantt')).toBeInTheDocument()
    expect(screen.getByTestId('mock-legend')).toBeInTheDocument()
    expect(screen.getByTestId('mock-detail-table')).toBeInTheDocument()
  })

  it('passes builds array to TimelineGanttChart', () => {
    const tc1 = makeTestCase({ name: 'a' })
    const tc2 = makeTestCase({ name: 'b', start: 1010000, stop: 1015000 })
    const builds = [makeBuild(2, [tc1]), makeBuild(1, [tc2])]
    render(
      <TimelineChart builds={builds} testCases={[tc1, tc2]} minStart={1000000} maxStop={1015000} />,
    )

    const gantt = screen.getByTestId('mock-gantt')
    expect(gantt.getAttribute('data-has-builds')).toBe('true')
    expect(gantt.getAttribute('data-builds-count')).toBe('2')
  })

  it('flattens all test cases across builds for minimap', () => {
    const tc1 = makeTestCase({ name: 'a' })
    const tc2 = makeTestCase({ name: 'b', start: 1010000, stop: 1015000 })
    const tc3 = makeTestCase({ name: 'c', start: 1020000, stop: 1025000 })
    const builds = [makeBuild(2, [tc1, tc2]), makeBuild(1, [tc3])]
    render(
      <TimelineChart
        builds={builds}
        testCases={[tc1, tc2, tc3]}
        minStart={1000000}
        maxStop={1025000}
      />,
    )

    const minimap = screen.getByTestId('mock-minimap')
    expect(minimap.getAttribute('data-tc-count')).toBe('3')
  })

  it('works without builds prop (backward compat)', () => {
    const testCases = [makeTestCase()]
    render(<TimelineChart testCases={testCases} minStart={1000000} maxStop={1020000} />)

    const gantt = screen.getByTestId('mock-gantt')
    expect(gantt.getAttribute('data-has-builds')).toBe('false')
    expect(gantt.getAttribute('data-tc-count')).toBe('1')
  })
})
