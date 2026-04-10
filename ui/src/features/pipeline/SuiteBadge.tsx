import { NavLink } from 'react-router'

import { getPassRateBadgeClass } from '@/lib/status-colors'
import { Badge } from '@/components/ui/badge'
import type { PipelineSuite } from '@/types/api'

interface SuiteBadgeProps {
  suite: PipelineSuite
}

export function SuiteBadge({ suite }: SuiteBadgeProps) {
  const statusIcon = suite.status === 'passed' ? '✓' : suite.status === 'degraded' ? '⚠' : '✗'

  return (
    <NavLink
      to={`/projects/${encodeURIComponent(suite.slug)}`}
      className="hover:bg-accent block rounded-lg border p-3 transition-colors"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-medium">{suite.slug}</span>
        <Badge className={getPassRateBadgeClass(suite.pass_rate)}>
          {statusIcon} {suite.pass_rate.toFixed(0)}%
        </Badge>
      </div>
      <div className="text-muted-foreground mt-1 flex gap-3 text-xs">
        <span>{suite.total} tests</span>
        {suite.failed > 0 && <span className="text-destructive">{suite.failed} failed</span>}
      </div>
    </NavLink>
  )
}
