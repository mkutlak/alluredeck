import { useState } from 'react'
import { NavLink } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, ExternalLink, GitBranch } from 'lucide-react'

import { formatDate, formatDuration } from '@/lib/utils'
import { getPassRateColorClass, STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import { formatProjectLabel } from '@/lib/projectLabel'
import { projectIndexOptions } from '@/lib/queries'
import { cn } from '@/lib/utils'
import { RunSuiteChips } from './RunSuiteChips'
import { RunFailures } from './RunFailures'
import type { PipelineRun } from '@/types/api'

export interface RunRowProps {
  run: PipelineRun
}

export function RunRow({ run }: RunRowProps) {
  const { aggregate } = run
  // Collapsed by default. Auto-expanding every failing run turned a page of
  // ten runs into an unscannable wall, and it fetched failures for all of them
  // before the user had asked for any.
  const [expanded, setExpanded] = useState(false)

  const shortSHA = run.commit_sha?.slice(0, 7)
  const failedCount = run.suites.reduce((sum, s) => sum + s.failed, 0)
  const hasFailures = failedCount > 0
  const suitesFailing = run.suites.filter((s) => s.failed > 0).length

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const projects = projectsResp?.data
  const groupProject =
    run.group_project_id != null
      ? projects?.find((p) => p.project_id === run.group_project_id)
      : undefined
  const groupLabel = groupProject ? formatProjectLabel(groupProject, projects) : run.group_slug
  const groupHref = run.group_project_id != null ? `/projects/${run.group_project_id}` : undefined

  const passRateClass = getPassRateColorClass(aggregate.pass_rate)
  const statusClass = hasFailures ? STATUS_TEXT_CLASSES.failed : STATUS_TEXT_CLASSES.passed

  return (
    <div className="px-4 py-2.5" data-testid="run-row">
      {/* Line 1 — identity and headline numbers, always one line. */}
      <div className="flex items-center gap-2 text-sm">
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground shrink-0"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
          aria-label={`Toggle failures for ${run.pipeline_id ?? shortSHA}`}
          data-testid="run-row-toggle"
        >
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        </button>

        <span aria-hidden="true" className={cn('shrink-0', statusClass)}>
          {hasFailures ? '✗' : '✓'}
        </span>

        <code className="shrink-0 font-semibold">
          {run.pipeline_url ?? run.ci_build_url ? (
            <a
              href={run.pipeline_url ?? run.ci_build_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 hover:underline"
            >
              {run.pipeline_id ?? shortSHA}
              <ExternalLink size={11} />
            </a>
          ) : (
            (run.pipeline_id ?? shortSHA)
          )}
        </code>

        {run.branch && (
          <span className="text-muted-foreground inline-flex shrink-0 items-center gap-1 text-xs">
            <GitBranch size={11} />
            {run.branch}
          </span>
        )}

        {groupLabel &&
          (groupHref ? (
            <NavLink
              to={groupHref}
              className="text-muted-foreground min-w-0 truncate text-xs hover:underline"
            >
              {groupLabel}
            </NavLink>
          ) : (
            <span className="text-muted-foreground min-w-0 truncate text-xs">{groupLabel}</span>
          ))}

        <span className="ml-auto flex shrink-0 items-center gap-3 text-xs">
          <span className={passRateClass}>{`${aggregate.pass_rate.toFixed(1)}%`}</span>
          <span className="text-muted-foreground">
            {/* Spell out "failing" — a bare "7/8 suites" reads as 7 passing. */}
            {hasFailures
              ? `${suitesFailing}/${aggregate.suites_total} suites failing · ${failedCount} failed tests`
              : `${aggregate.suites_total}/${aggregate.suites_total} suites passed`}
          </span>
          <span className="text-muted-foreground tabular-nums">
            {formatDuration(aggregate.total_duration_ms)}
          </span>
          <span className="text-muted-foreground tabular-nums">{formatDate(run.timestamp)}</span>
        </span>
      </div>

      {/* Line 2 — only when something failed, so a green run stays one line. */}
      {hasFailures && (
        <div className="mt-1.5 pl-6">
          <RunSuiteChips suites={run.suites} />
        </div>
      )}

      {expanded && (
        <div className="mt-2 pl-6">
          <RunFailures run={run} />
        </div>
      )}
    </div>
  )
}
