import { useMemo } from 'react'
import type { TimelineTestCase } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'

const LEGEND_ITEMS = [
  { status: 'passed', label: 'Passed' },
  { status: 'failed', label: 'Failed' },
  { status: 'broken', label: 'Broken' },
  { status: 'skipped', label: 'Skipped' },
] as const

export interface TimelineLegendProps {
  testCases: TimelineTestCase[]
  statusColors: StatusColorMap
}

export function TimelineLegend({ testCases, statusColors }: TimelineLegendProps) {
  const presentStatuses = useMemo(
    () => LEGEND_ITEMS.filter(({ status }) => testCases.some((tc) => tc.status === status)),
    [testCases],
  )

  return (
    <div data-testid="legend" className="flex flex-wrap gap-4">
      {presentStatuses.map(({ status, label }) => (
        <div key={status} className="flex items-center gap-1.5">
          <span
            data-testid={`legend-swatch-${status}`}
            className="inline-block h-3 w-3 rounded-sm"
            style={{ backgroundColor: statusColors[status as keyof StatusColorMap] ?? '#8c8fa1' }}
          />
          <span className="text-muted-foreground text-xs">{label}</span>
        </div>
      ))}
    </div>
  )
}
