import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { TimelineBuildEntry, TimelineTestCase } from '@/types/api'
import { TimelineChart } from '../TimelineChart'

function propsProbe(testId: string) {
  return (props: { testCases: unknown[]; builds?: unknown[] }) => (
    <div
      data-testid={testId}
      data-tc-count={props.testCases.length}
      data-builds-count={props.builds?.length ?? 'none'}
    />
  )
}

vi.mock('../TimelineMinimap', () => ({ TimelineMinimap: propsProbe('mock-minimap') }))
vi.mock('../TimelineGanttChart', () => ({ TimelineGanttChart: propsProbe('mock-gantt') }))
vi.mock('../TimelineLegend', () => ({ TimelineLegend: propsProbe('mock-legend') }))
vi.mock('../TimelineDetailTable', () => ({ TimelineDetailTable: propsProbe('mock-detail-table') }))
vi.mock('@/hooks/useContainerWidth', () => ({ useContainerWidth: () => 1000 }))

function makeTestCase(name: string, start: number): TimelineTestCase {
  return {
    name,
    full_name: `com.example.${name}`,
    status: 'passed',
    start,
    stop: start + 5000,
    duration: 5000,
    thread: 'main',
    host: 'host-1',
  }
}

describe('TimelineChart', () => {
  it('forwards all test cases to every panel and the builds to the gantt chart', () => {
    const testCases = [makeTestCase('alpha', 1_000_000), makeTestCase('beta', 1_010_000)]
    const builds = [{ build_order: 2 }, { build_order: 1 }] as TimelineBuildEntry[]
    render(
      <TimelineChart
        testCases={testCases}
        minStart={1_000_000}
        maxStop={1_015_000}
        builds={builds}
      />,
    )

    for (const id of ['mock-minimap', 'mock-gantt', 'mock-legend', 'mock-detail-table']) {
      expect(screen.getByTestId(id)).toHaveAttribute('data-tc-count', '2')
    }
    expect(screen.getByTestId('mock-gantt')).toHaveAttribute('data-builds-count', '2')
  })
})
