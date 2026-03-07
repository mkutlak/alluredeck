import { useMemo } from 'react'
import type { TimelineTestCase } from '@/types/api'
import { STATUS_COLORS, detectLaneStrategy, toTimelineLanes } from '@/lib/chart-utils'
import { formatDuration, getStatusVariant } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
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

  const laneData = useMemo(
    () =>
      lanes.map((lane) => {
        const laneTcs = testCases.filter((tc) => {
          if (strategy === 'thread') return (tc.thread || 'default') === lane.id
          if (strategy === 'host') return (tc.host || 'default') === lane.id
          return true
        })
        const bars = laneTcs.map((tc) => computeBar(tc, minStart, totalMs))
        const rows = stackBarsIntoRows(bars)
        return { lane, rows }
      }),
    [lanes, testCases, strategy, minStart, totalMs],
  )

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
          <div className="relative h-5 border-b border-border">
            {ticks.map((tick) => (
              <span
                key={tick.ms}
                className="absolute top-0 text-[11px] leading-none text-muted-foreground whitespace-nowrap"
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
            className="grid border-b border-border/50 last:border-b-0"
            style={{ gridTemplateColumns: `${LABEL_WIDTH} 1fr` }}
          >
            {/* Lane label */}
            <div className="flex items-start pt-1 pr-2">
              <span className="truncate text-xs font-medium text-muted-foreground">
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
                      <TooltipContent className="max-w-[260px] border bg-popover p-2 text-popover-foreground">
                        <p className="max-w-[240px] truncate text-xs font-medium">
                          {bar.tc.name}
                        </p>
                        <div className="mt-1 flex items-center gap-2">
                          <Badge variant={getStatusVariant(bar.tc.status)} className="text-xs">
                            {bar.tc.status}
                          </Badge>
                          <span className="text-xs text-muted-foreground">
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
              <span className="text-xs text-muted-foreground">{label}</span>
            </div>
          ))}
        </div>
      </div>
    </TooltipProvider>
  )
}
