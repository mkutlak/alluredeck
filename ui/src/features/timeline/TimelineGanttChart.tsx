import { useRef, useState, useMemo, useEffect, useCallback } from 'react'
import { scaleLinear } from 'd3-scale'
import { zoom as d3Zoom, zoomIdentity } from 'd3-zoom'
import { select } from 'd3-selection'
import type { TimelineTestCase, TimelineBuildEntry } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'
import { computeGanttLayout, computeMultiBuildLayout } from './timelineGanttHelpers'
import { computeTicks } from './timelineHelpers'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const BAR_HEIGHT = 6
const BAR_GAP = 2
const BAND_GAP = 24
const MARGIN = { top: 4, right: 8, bottom: 4, left: 8 }
const AXIS_HEIGHT = 20

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface TimelineGanttChartProps {
  testCases: TimelineTestCase[]
  minStart: number
  maxStop: number
  statusColors: StatusColorMap
  width: number
  height?: number
  selectedRange: [number, number] | null
  onViewportChange: (range: [number, number]) => void
  onBrushSelect: (range: [number, number] | null) => void
  highlightedTestId: string | null
  /** When provided with more than 1 entry, enables multi-build stacked layout. */
  builds?: TimelineBuildEntry[]
}

// ---------------------------------------------------------------------------
// ZoomTransform shape (matches d3-zoom's ZoomTransform)
// ---------------------------------------------------------------------------

interface ZoomState {
  k: number
  x: number
  y: number
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBandDate(iso: string): string {
  return iso.slice(0, 10)
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TimelineGanttChart({
  testCases,
  minStart,
  maxStop,
  statusColors,
  width,
  height = 450,
  selectedRange: _selectedRange,
  onViewportChange,
  onBrushSelect: _onBrushSelect,
  highlightedTestId,
  builds,
}: TimelineGanttChartProps) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [zoomTransform, setZoomTransform] = useState<ZoomState>({ k: 1, x: 0, y: 0 })

  const innerWidth = width - MARGIN.left - MARGIN.right

  const isMultiBuild = builds !== undefined && builds.length > 1

  // Zoomed domain computed from the zoom transform
  const zoomedDomain = useMemo(() => {
    const domainSpan = maxStop - minStart
    const zoomedSpan = domainSpan / zoomTransform.k
    const zoomedMin = minStart - (zoomTransform.x / innerWidth) * zoomedSpan
    const zoomedMax = zoomedMin + zoomedSpan
    return [zoomedMin, zoomedMax] as const
  }, [minStart, maxStop, zoomTransform, innerWidth])

  // Zoomed x scale: new linear scale with the zoomed domain
  const zoomedXScale = useMemo(
    () =>
      scaleLinear()
        .domain([zoomedDomain[0], zoomedDomain[1]])
        .range([MARGIN.left, width - MARGIN.right]),
    [zoomedDomain, width],
  )

  // Single-build layout (backward compat)
  const singleLayout = useMemo(
    () =>
      isMultiBuild
        ? null
        : computeGanttLayout(testCases, (ms) => zoomedXScale(ms) ?? 0, BAR_HEIGHT, BAR_GAP),
    [testCases, zoomedXScale, isMultiBuild],
  )

  // Multi-build layout
  const multiBuildResult = useMemo(
    () =>
      isMultiBuild
        ? computeMultiBuildLayout(
            builds,
            (ms) => zoomedXScale(ms) ?? 0,
            BAR_HEIGHT,
            BAR_GAP,
            BAND_GAP,
          )
        : null,
    [builds, zoomedXScale, isMultiBuild],
  )

  // Compute ticks using the zoomed absolute timestamps
  const ticks = useMemo(() => computeTicks(zoomedDomain[0], zoomedDomain[1]), [zoomedDomain])

  // SVG height adapts to content but caps at the height prop
  const contentHeight = isMultiBuild
    ? Math.max(200, multiBuildResult?.totalHeight ?? 200)
    : Math.max(200, singleLayout?.totalHeight ?? 200)

  const svgHeight = Math.min(height, MARGIN.top + AXIS_HEIGHT + contentHeight + MARGIN.bottom)

  // Attach D3 zoom behavior
  useEffect(() => {
    if (!svgRef.current || width <= 0) return

    const zoomBehavior = d3Zoom<SVGSVGElement, unknown>()
      .scaleExtent([1, 50])
      .translateExtent([
        [0, 0],
        [width, svgHeight],
      ])
      .filter((event: Event) => {
        // Allow wheel + drag, but not right-click
        if (event instanceof MouseEvent && event.button === 2) return false
        return true
      })
      .on('zoom', (event) => {
        const t = event.transform as ZoomState
        setZoomTransform({ k: t.k, x: t.x, y: t.y })

        // Notify parent of the viewport range
        const domainSpan = maxStop - minStart
        const zoomedSpan = domainSpan / t.k
        const zoomedMin = minStart - (t.x / innerWidth) * zoomedSpan
        onViewportChange([zoomedMin, zoomedMin + zoomedSpan])
      })

    const el = svgRef.current
    select(el).call(zoomBehavior)

    return () => {
      select(el).on('.zoom', null)
    }
  }, [width, svgHeight, minStart, maxStop, innerWidth, onViewportChange])

  // Keyboard handler for pan/zoom
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const PAN_STEP = innerWidth * 0.1
      const ZOOM_FACTOR = 1.2

      switch (e.key) {
        case 'ArrowLeft':
          e.preventDefault()
          setZoomTransform((prev) => ({ ...prev, x: prev.x + PAN_STEP }))
          break
        case 'ArrowRight':
          e.preventDefault()
          setZoomTransform((prev) => ({ ...prev, x: prev.x - PAN_STEP }))
          break
        case '+':
        case '=':
          e.preventDefault()
          setZoomTransform((prev) => ({
            ...prev,
            k: Math.min(50, prev.k * ZOOM_FACTOR),
          }))
          break
        case '-':
          e.preventDefault()
          setZoomTransform((prev) => ({
            ...prev,
            k: Math.max(1, prev.k / ZOOM_FACTOR),
          }))
          break
        case 'Home':
          e.preventDefault()
          setZoomTransform({ k: zoomIdentity.k, x: zoomIdentity.x, y: zoomIdentity.y })
          break
        case 'Escape':
          e.preventDefault()
          _onBrushSelect(null)
          break
      }
    },
    [innerWidth, _onBrushSelect],
  )

  return (
    <svg
      ref={svgRef}
      data-testid="gantt-chart"
      role="img"
      aria-label="Test execution timeline"
      width={width}
      height={svgHeight}
      tabIndex={0}
      onKeyDown={handleKeyDown}
      className="bg-card focus-visible:ring-ring rounded border outline-none focus-visible:ring-2"
    >
      <defs>
        <clipPath id="gantt-clip">
          <rect
            x={MARGIN.left}
            y={MARGIN.top + AXIS_HEIGHT}
            width={innerWidth}
            height={contentHeight}
          />
        </clipPath>
      </defs>

      {/* Time axis */}
      <g data-testid="time-axis">
        {ticks.map((tick) => {
          const x = zoomedXScale(zoomedDomain[0] + tick.ms) ?? 0
          return (
            <g key={tick.ms}>
              <line
                x1={x}
                y1={MARGIN.top + AXIS_HEIGHT}
                x2={x}
                y2={svgHeight - MARGIN.bottom}
                stroke="currentColor"
                opacity={0.1}
              />
              <text
                x={x}
                y={MARGIN.top + AXIS_HEIGHT - 4}
                className="fill-muted-foreground"
                fontSize={10}
              >
                {tick.label}
              </text>
            </g>
          )
        })}
      </g>

      {/* Bars */}
      <g clipPath="url(#gantt-clip)">
        {isMultiBuild && multiBuildResult
          ? multiBuildResult.bands.map((band, bandIdx) => (
              <g key={band.buildOrder}>
                {/* Band label */}
                <text
                  x={MARGIN.left + 4}
                  y={MARGIN.top + AXIS_HEIGHT + band.yOffset - 6}
                  className="fill-muted-foreground"
                  fontSize={10}
                  fontWeight={500}
                >
                  Build #{band.buildOrder} — {formatBandDate(band.createdAt)}
                </text>

                {/* Separator line (between bands, not before the first) */}
                {bandIdx > 0 && (
                  <line
                    data-testid="band-separator"
                    x1={MARGIN.left}
                    y1={MARGIN.top + AXIS_HEIGHT + band.yOffset - BAND_GAP / 2}
                    x2={width - MARGIN.right}
                    y2={MARGIN.top + AXIS_HEIGHT + band.yOffset - BAND_GAP / 2}
                    stroke="currentColor"
                    opacity={0.15}
                    strokeDasharray="4 2"
                  />
                )}

                {/* Bars within band */}
                {band.layout.bars.map((bar, i) => (
                  <rect
                    key={`${bar.tc.full_name}-${bar.tc.start}-${i}`}
                    data-testid="gantt-bar"
                    x={bar.x}
                    y={bar.y + MARGIN.top + AXIS_HEIGHT + band.yOffset}
                    width={Math.max(2, bar.width)}
                    height={BAR_HEIGHT}
                    rx={1}
                    fill={
                      statusColors[bar.tc.status as keyof StatusColorMap] ?? statusColors.skipped
                    }
                    opacity={highlightedTestId === bar.tc.full_name ? 1 : 0.85}
                    className="cursor-pointer transition-opacity"
                  />
                ))}
              </g>
            ))
          : /* Single-build mode */
            singleLayout?.bars.map((bar, i) => (
              <rect
                key={`${bar.tc.full_name}-${bar.tc.start}-${i}`}
                data-testid="gantt-bar"
                x={bar.x}
                y={bar.y + MARGIN.top + AXIS_HEIGHT}
                width={Math.max(2, bar.width)}
                height={BAR_HEIGHT}
                rx={1}
                fill={statusColors[bar.tc.status as keyof StatusColorMap] ?? statusColors.skipped}
                opacity={highlightedTestId === bar.tc.full_name ? 1 : 0.85}
                className="cursor-pointer transition-opacity"
              />
            ))}
      </g>
    </svg>
  )
}
