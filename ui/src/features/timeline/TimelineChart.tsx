import { useMemo } from 'react'
import type { TimelineTestCase } from '@/types/api'
import { STATUS_COLORS, detectLaneStrategy, toTimelineLanes } from '@/lib/chart-utils'
import { formatDuration, getStatusVariant } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { computeTicks, computeBar, stackBarsIntoRows } from './timelineHelpers'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface TimelineChartProps {
  testCases: TimelineTestCase[]
  minStart: number
  maxStop: number
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const LEGEND_ITEMS = [
  { status: 'passed', label: 'Passed' },
  { status: 'failed', label: 'Failed' },
  { status: 'broken', label: 'Broken' },
  { status: 'skipped', label: 'Skipped' },
] as const

const LABEL_WIDTH = '8rem'

function statusColor(status: string): string {
  return STATUS_COLORS[status as keyof typeof STATUS_COLORS] ?? STATUS_COLORS.skipped
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TimelineChart({ testCases, minStart, maxStop }: TimelineChartProps) {
  const totalMs = Math.max(maxStop - minStart, 1)
  const ticks = computeTicks(minStart, maxStop)

  const strategy = detectLaneStrategy(testCases)
  const lanes = toTimelineLanes(testCases, strategy)

  const laneData = useMemo(() => {
    const index = new Map<string, TimelineTestCase[]>()
    for (const tc of testCases) {
      const key =
        strategy === 'thread'
          ? (tc.thread || 'default')
          : strategy === 'host'
            ? (tc.host || 'default')
            : 'default'
      const arr = index.get(key)
      if (arr) arr.push(tc)
      else index.set(key, [tc])
    }
    return lanes.map((lane) => {
      const laneTcs = index.get(lane.id) ?? []
      const bars = laneTcs.map((tc) => computeBar(tc, minStart, totalMs))
      const rows = stackBarsIntoRows(bars)
      return { lane, rows }
    })
  }, [lanes, testCases, strategy, minStart, totalMs])

  const presentStatuses = useMemo(
    () => LEGEND_ITEMS.filter(({ status }) => testCases.some((tc) => tc.status === status)),
    [testCases],
  )

  return (
    <TooltipProvider>
      <div className="w-full select-none">
        {/* ── Time axis ──────────────────────────────────────────────── */}
        <div
          data-testid="time-axis"
          className="grid pb-1"
          style={{ gridTemplateColumns: `${LABEL_WIDTH} 1fr` }}
        >
          {/* gutter */}
          <div />
          {/* tick labels */}
          <div className="border-border relative h-5 border-b">
            {ticks.map((tick) => (
              <span
                key={tick.ms}
                className="text-muted-foreground absolute top-0 text-[11px] leading-none whitespace-nowrap"
                style={{ left: `${(tick.ms / totalMs) * 100}%` }}
              >
                {tick.label}
              </span>
            ))}
          </div>
        </div>

        {/* ── Lanes ──────────────────────────────────────────────────── */}
        {laneData.map(({ lane, rows }) => (
          <div
            key={lane.id}
            className="border-border/50 grid border-b last:border-b-0"
            style={{ gridTemplateColumns: `${LABEL_WIDTH} 1fr` }}
          >
            {/* Lane label */}
            <div className="flex items-start pt-1 pr-2">
              <span className="text-muted-foreground truncate text-xs font-medium">
                {lane.label}
              </span>
            </div>

            {/* Bar area */}
            <div>
              {rows.map((row, rowIdx) => (
                <div key={rowIdx} className="relative h-7">
                  {row.map((bar) => (
                    <Tooltip key={`${bar.tc.name}-${bar.tc.start}`}>
                      <TooltipTrigger asChild>
                        <div
                          data-testid={`bar-${bar.tc.name}`}
                          className="absolute top-1 h-5 cursor-pointer rounded-sm opacity-90 transition-opacity hover:opacity-100"
                          style={{
                            left: `${bar.leftPct}%`,
                            width: `${bar.widthPct}%`,
                            backgroundColor: statusColor(bar.tc.status),
                          }}
                        />
                      </TooltipTrigger>
                      <TooltipContent className="bg-popover text-popover-foreground max-w-[260px] border p-2">
                        <p className="max-w-[240px] truncate text-xs font-medium">{bar.tc.name}</p>
                        <div className="mt-1 flex items-center gap-2">
                          <Badge variant={getStatusVariant(bar.tc.status)} className="text-xs">
                            {bar.tc.status}
                          </Badge>
                          <span className="text-muted-foreground text-xs">
                            {formatDuration(bar.tc.duration)}
                          </span>
                        </div>
                      </TooltipContent>
                    </Tooltip>
                  ))}
                </div>
              ))}
            </div>
          </div>
        ))}

        {/* ── Legend ─────────────────────────────────────────────────── */}
        <div data-testid="legend" className="mt-3 flex flex-wrap gap-4">
          {presentStatuses.map(({ status, label }) => (
            <div key={status} className="flex items-center gap-1.5">
              <span
                className="inline-block h-3 w-3 rounded-sm"
                style={{ backgroundColor: statusColor(status) }}
              />
              <span className="text-muted-foreground text-xs">{label}</span>
            </div>
          ))}
        </div>
      </div>
    </TooltipProvider>
  )
}
