import { useState } from 'react'
import { ChevronDown, ChevronRight, ExternalLink, GitBranch, GitCommitHorizontal } from 'lucide-react'

import { formatDate, formatDuration, formatPassRate } from '@/lib/utils'
import { getPassRateColorClass, STATUS_TEXT_CLASSES } from '@/lib/status-colors'
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
  const shortSHA = run.commit_sha?.slice(0, 7)
  const hasPipeline = !!run.pipeline_id
  // Nothing ran (every suite skipped): "—" in neutral ink, not a red 0%. Suite
  // status, not zero counts: a build with unreadable stats also has total 0.
  const allSkipped = run.suites.length > 0 && run.suites.every((s) => s.status === 'skipped')
  const rateClass = allSkipped
    ? STATUS_TEXT_CLASSES.skipped
    : getPassRateColorClass(aggregate.pass_rate)

  return (
    <Card>
      <CardHeader className="p-4 pb-2">
        <button
          className="flex w-full items-center gap-2 text-left"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}

          {hasPipeline ? (
            <div className="flex items-center gap-2">
              <code className="text-sm font-semibold">
                {run.pipeline_url ? (
                  <a
                    href={run.pipeline_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 hover:underline"
                    onClick={(e) => e.stopPropagation()}
                  >
                    Pipeline {run.pipeline_id}
                    <ExternalLink size={12} />
                  </a>
                ) : (
                  <>Pipeline {run.pipeline_id}</>
                )}
              </code>
              {shortSHA && (
                <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
                  <GitCommitHorizontal size={12} />
                  {shortSHA}
                </span>
              )}
            </div>
          ) : (
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
          )}

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
          <span className={rateClass}>
            {aggregate.suites_passed}/{aggregate.suites_total} suites passing
          </span>
          {' · '}
          <span className={rateClass}>
            {formatPassRate(aggregate.tests_passed, aggregate.tests_total, aggregate.tests_skipped)}{' '}
            overall
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
