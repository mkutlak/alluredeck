import type { TimelineTestCase } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'
import { Badge } from '@/components/ui/badge'
import { formatDuration, getStatusVariant } from '@/lib/utils'

export interface GanttTooltipProps {
  testCase: TimelineTestCase | null
  position: { x: number; y: number } | null
  statusColors: StatusColorMap
}

export function GanttTooltip({ testCase, position, statusColors: _statusColors }: GanttTooltipProps) {
  if (!testCase || !position) return null

  return (
    <div
      data-testid="gantt-tooltip"
      className="bg-popover text-popover-foreground pointer-events-none absolute z-50 max-w-xs rounded-md border p-2 shadow-md"
      style={{ left: position.x, top: position.y }}
    >
      <p className="truncate text-xs font-medium">{testCase.name}</p>
      <p className="text-muted-foreground truncate text-[10px]">{testCase.full_name}</p>
      <div className="mt-1 flex items-center gap-2">
        <Badge variant={getStatusVariant(testCase.status)} className="text-xs">
          {testCase.status}
        </Badge>
        <span className="text-muted-foreground text-xs">{formatDuration(testCase.duration)}</span>
      </div>
      {testCase.thread && (
        <p className="text-muted-foreground mt-1 text-[10px]">Worker: {testCase.thread}</p>
      )}
      {testCase.host && (
        <p className="text-muted-foreground text-[10px]">Host: {testCase.host}</p>
      )}
    </div>
  )
}
