import { useRef, useState, useCallback, useMemo } from 'react'
import type { TimelineTestCase } from '@/types/api'
import { useStatusColors } from '@/hooks/useStatusColors'
import { useContainerWidth } from '@/hooks/useContainerWidth'
import { filterByTimeRange } from './timelineGanttHelpers'
import { TimelineMinimap } from './TimelineMinimap'
import { TimelineGanttChart } from './TimelineGanttChart'
import { TimelineLegend } from './TimelineLegend'
import { TimelineDetailTable } from './TimelineDetailTable'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface TimelineChartProps {
  testCases: TimelineTestCase[]
  minStart: number
  maxStop: number
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TimelineChart({ testCases, minStart, maxStop }: TimelineChartProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const width = useContainerWidth(containerRef)

  const statusColors = useStatusColors()

  const [brushRange, setBrushRange] = useState<[number, number] | null>(null)
  const [viewportRange, setViewportRange] = useState<[number, number] | null>(null)
  const [highlightedTestId, setHighlightedTestId] = useState<string | null>(null)

  // Filter tests for detail table based on brush selection
  const tableTestCases = useMemo(
    () => (brushRange ? filterByTimeRange(testCases, brushRange[0], brushRange[1]) : testCases),
    [testCases, brushRange],
  )

  const handleTestClick = useCallback((tc: TimelineTestCase) => {
    setHighlightedTestId(tc.full_name)
  }, [])

  return (
    <div ref={containerRef} className="space-y-3">
      {width > 0 && (
        <>
          <TimelineMinimap
            testCases={testCases}
            minStart={minStart}
            maxStop={maxStop}
            statusColors={statusColors}
            width={width}
            onBrushChange={setBrushRange}
            viewportRange={viewportRange}
          />
          <TimelineGanttChart
            testCases={testCases}
            minStart={minStart}
            maxStop={maxStop}
            statusColors={statusColors}
            width={width}
            selectedRange={brushRange}
            onViewportChange={setViewportRange}
            onBrushSelect={setBrushRange}
            highlightedTestId={highlightedTestId}
          />
        </>
      )}
      <TimelineLegend testCases={testCases} statusColors={statusColors} />
      <TimelineDetailTable
        testCases={tableTestCases}
        statusColors={statusColors}
        onTestClick={handleTestClick}
      />
    </div>
  )
}
