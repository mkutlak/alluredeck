import { useState } from 'react'
import { ChevronDown, ChevronRight, ExternalLink, GitBranch } from 'lucide-react'

import { formatDate, formatDuration } from '@/lib/utils'
import { getPassRateColorClass } from '@/lib/status-colors'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { SuiteBadge } from './SuiteBadge'
import type { PipelineRun } from '@/types/api'

interface PipelineRunCardProps {
  run: PipelineRun
}

export function PipelineRunCard({ run }: PipelineRunCardProps) {
  const [expanded, setExpanded] = useState(false)
  const { aggregate } = run
  const shortSHA = run.commit_sha.slice(0, 7)

  return (
    <Card>
      <CardHeader className="p-4 pb-2">
        <button
          className="flex w-full items-center gap-2 text-left"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}

          <code className="text-sm font-semibold">
            {run.ci_build_url ? (
              <a
                href={run.ci_build_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {shortSHA}
                <ExternalLink size={12} />
              </a>
            ) : (
              shortSHA
            )}
          </code>

          {run.branch && (
            <Badge variant="outline" className="gap-1 text-xs font-normal">
              <GitBranch size={12} />
              {run.branch}
            </Badge>
          )}

          <span className="text-muted-foreground ml-auto text-xs">{formatDate(run.timestamp)}</span>
        </button>
      </CardHeader>

      <CardContent className="px-4 pt-0 pb-4">
        {/* Summary line */}
        <p className="text-muted-foreground text-sm">
          <span className={getPassRateColorClass(aggregate.pass_rate)}>
            {aggregate.suites_passed}/{aggregate.suites_total} suites passing
          </span>
          {' · '}
          <span className={getPassRateColorClass(aggregate.pass_rate)}>
            {aggregate.pass_rate.toFixed(1)}% overall
          </span>
          {' · '}
          {formatDuration(aggregate.total_duration_ms)}
        </p>

        {/* Expanded suite grid */}
        {expanded && (
          <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {run.suites.map((suite) => (
              <SuiteBadge key={suite.project_id} suite={suite} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
