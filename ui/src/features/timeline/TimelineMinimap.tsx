import { useRef, useEffect, useMemo, useCallback } from 'react'
import { scaleLinear } from 'd3-scale'
import { brushX } from 'd3-brush'
import { select } from 'd3-selection'
import type { TimelineTestCase } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'
import { computeMinimapBars } from './timelineGanttHelpers'

const MINIMAP_HEIGHT = 40
const BAR_HEIGHT = 2

export interface TimelineMinimapProps {
  testCases: TimelineTestCase[]
  minStart: number
  maxStop: number
  statusColors: StatusColorMap
  width: number
  onBrushChange: (range: [number, number] | null) => void
  viewportRange: [number, number] | null
}

export function TimelineMinimap({
  testCases,
  minStart,
  maxStop,
  statusColors,
  width,
  onBrushChange,
  viewportRange,
}: TimelineMinimapProps) {
  const brushRef = useRef<SVGGElement>(null)

  const xScale = useMemo(
    () => scaleLinear().domain([minStart, maxStop]).range([0, width]),
    [minStart, maxStop, width],
  )

  const bars = useMemo(
    () => computeMinimapBars(testCases, (ms) => xScale(ms) ?? 0, MINIMAP_HEIGHT),
    [testCases, xScale],
  )

  // Attach D3 brush
  useEffect(() => {
    const el = brushRef.current
    if (!el || width <= 0) return

    const brush = brushX()
      .extent([
        [0, 0],
        [width, MINIMAP_HEIGHT],
      ])
      .on('end', (event) => {
        if (!event.selection) {
          onBrushChange(null)
          return
        }
        const [x0, x1] = event.selection as [number, number]
        onBrushChange([xScale.invert(x0), xScale.invert(x1)])
      })

    select(el).call(brush)

    return () => {
      select(el).selectAll('*').attr('pointer-events', 'none')
    }
  }, [width, xScale, onBrushChange])

  // Sync viewport range to brush position
  useEffect(() => {
    if (!brushRef.current || !viewportRange || width <= 0) return
    // Only sync programmatically when needed — could add brush.move here
  }, [viewportRange, width, xScale])

  const statusColor = useCallback(
    (status: string) => statusColors[status as keyof StatusColorMap] ?? statusColors.skipped,
    [statusColors],
  )

  return (
    <svg
      data-testid="timeline-minimap"
      width={width}
      height={MINIMAP_HEIGHT}
      className="bg-muted/30 rounded border"
    >
      {bars.map((bar, i) => (
        <rect
          key={`${bar.tc.name}-${bar.tc.start}-${i}`}
          data-testid="minimap-bar"
          x={bar.x}
          y={bar.y}
          width={Math.max(1, bar.width)}
          height={BAR_HEIGHT}
          fill={statusColor(bar.tc.status)}
          opacity={0.8}
        />
      ))}
      <g ref={brushRef} data-testid="minimap-brush" />
    </svg>
  )
}
