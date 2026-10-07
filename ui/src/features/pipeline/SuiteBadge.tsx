import { NavLink } from 'react-router'

import { getPassRateBadgeClass } from '@/lib/status-colors'
import { formatPassRate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import type { PipelineSuite } from '@/types/api'

const STATUS_ICON = { passed: '✓', degraded: '⚠', failed: '✗', skipped: '○' } as const

interface SuiteBadgeProps {
  suite: PipelineSuite
}

export function SuiteBadge({ suite }: SuiteBadgeProps) {
  const statusIcon = STATUS_ICON[suite.status]
  const isSkipped = suite.status === 'skipped'
  const skipped = suite.skipped ?? 0
  // The API's `passed` is exact; without it, broken is inside `failed`, so
  // total - failed - skipped still counts only tests that passed.
  const passed = suite.passed ?? suite.total - suite.failed - skipped

  return (
    <NavLink
      to={`/projects/${encodeURIComponent(suite.project_id)}`}
      className="hover:bg-accent block rounded-lg border p-3 transition-colors"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-medium">{suite.slug}</span>
        {/* Nothing ran: the neutral skipped badge, not the pass-rate colours (whose
            fallthrough for a 0% is the loud default variant). */}
        <Badge
          variant={isSkipped ? 'skipped' : 'default'}
          className={isSkipped ? undefined : getPassRateBadgeClass(suite.pass_rate)}
        >
          {statusIcon} {formatPassRate(passed, suite.total, skipped)}
        </Badge>
      </div>
      <div className="text-muted-foreground mt-1 flex gap-3 text-xs">
        <span>{suite.total} tests</span>
        {suite.failed > 0 && <span className="text-destructive">{suite.failed} failed</span>}
      </div>
    </NavLink>
  )
}
