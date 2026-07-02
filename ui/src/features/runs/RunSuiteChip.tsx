import { NavLink } from 'react-router'
import { Badge } from '@/components/ui/badge'
import type { PipelineSuite } from '@/types/api'

export interface RunSuiteChipProps {
  suite: PipelineSuite
}

export function RunSuiteChip({ suite }: RunSuiteChipProps) {
  const statusIcon = suite.status === 'passed' ? '✓' : suite.status === 'degraded' ? '⚠' : '✗'

  return (
    <NavLink
      to={`/projects/${suite.project_id}/reports/${suite.build_number}`}
      className="hover:bg-accent inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors"
      data-testid="run-suite-chip"
    >
      <span aria-hidden="true">{statusIcon}</span>
      <span className="font-medium">{suite.slug}</span>
      {suite.failed > 0 && (
        <Badge variant="failed" className="px-1.5 py-0 text-[10px]">
          {suite.failed}
        </Badge>
      )}
    </NavLink>
  )
}
