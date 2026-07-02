import { useState } from 'react'
import { NavLink } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, ExternalLink, GitBranch, GitCommitHorizontal } from 'lucide-react'

import { formatDate, formatDuration } from '@/lib/utils'
import { getPassRateColorClass } from '@/lib/status-colors'
import { formatProjectLabel } from '@/lib/projectLabel'
import { projectIndexOptions } from '@/lib/queries'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { RunSuiteChip } from './RunSuiteChip'
import { RunFailures } from './RunFailures'
import type { PipelineRun } from '@/types/api'

export interface RunRowProps {
  run: PipelineRun
}

export function RunRow({ run }: RunRowProps) {
  const { aggregate } = run
  const [expanded, setExpanded] = useState(aggregate.suites_passed < aggregate.suites_total)
  const shortSHA = run.commit_sha?.slice(0, 7)
  const hasPipeline = !!run.pipeline_id

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const projects = projectsResp?.data
  const groupProject =
    run.group_project_id != null
      ? projects?.find((p) => p.project_id === run.group_project_id)
      : undefined
  const groupLabel = groupProject ? formatProjectLabel(groupProject, projects) : run.group_slug
  const groupHref = run.group_project_id != null ? `/projects/${run.group_project_id}` : undefined

  return (
    <Card data-testid="run-row">
      <CardHeader className="p-4 pb-2">
        <button
          className="flex w-full flex-wrap items-center gap-2 text-left"
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

          {groupLabel &&
            (groupHref ? (
              <NavLink
                to={groupHref}
                className="text-muted-foreground text-xs hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {groupLabel}
              </NavLink>
            ) : (
              <span className="text-muted-foreground text-xs">{groupLabel}</span>
            ))}

          <span className="text-muted-foreground ml-auto text-xs">{formatDate(run.timestamp)}</span>
        </button>
      </CardHeader>

      <CardContent className="px-4 pt-0 pb-4">
        <p className="text-muted-foreground text-sm">
          <span className={getPassRateColorClass(aggregate.pass_rate)}>
            {aggregate.suites_passed}/{aggregate.suites_total} suites
          </span>
          {' · '}
          <span className={getPassRateColorClass(aggregate.pass_rate)}>
            {aggregate.tests_passed}/{aggregate.tests_total} tests
          </span>
          {' · '}
          <span className={getPassRateColorClass(aggregate.pass_rate)}>
            {aggregate.pass_rate.toFixed(1)}%
          </span>
          {' · '}
          {formatDuration(aggregate.total_duration_ms)}
        </p>

        <div className="mt-3 flex flex-wrap gap-2">
          {run.suites.map((suite) => (
            <RunSuiteChip key={suite.project_id} suite={suite} />
          ))}
        </div>

        {expanded && <RunFailures run={run} />}
      </CardContent>
    </Card>
  )
}
