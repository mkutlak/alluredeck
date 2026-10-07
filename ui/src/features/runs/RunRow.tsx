import { useState } from 'react'
import { NavLink } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, ExternalLink, GitBranch } from 'lucide-react'

import { formatDate, formatDuration, formatPassRate } from '@/lib/utils'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
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
  // A suite whose report could not be read has no counts but status "failed":
  // it fails the run although no test is listed as failed.
  const suitesFailing = run.suites.filter((s) => s.failed > 0 || s.status === 'failed').length
  const isFailing = suitesFailing > 0

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const projects = projectsResp?.data
  const groupProject =
    run.group_project_id != null
      ? projects?.find((p) => p.project_id === run.group_project_id)
      : undefined
  const groupLabel = groupProject ? formatProjectLabel(groupProject, projects) : run.group_slug
  const groupHref = run.group_project_id != null ? `/projects/${run.group_project_id}` : undefined

  // Every suite skipped: nothing ran, so there is no verdict and no rate. Neutral
  // glyph, "—", never the green ✓ or a red 0%. Decided from suite status, not
  // from zero counts, which also describe a build whose stats were unreadable.
  const allSkipped = run.suites.length > 0 && run.suites.every((s) => s.status === 'skipped')
  const verdict = isFailing ? 'failed' : allSkipped ? 'skipped' : 'passed'
  const glyph = { failed: '✗', skipped: '○', passed: '✓' }[verdict]
  const passRate = formatPassRate(
    aggregate.tests_passed,
    aggregate.tests_total,
    aggregate.tests_skipped,
  )

  return (
    <div className="px-4 py-2.5" data-testid="run-row">
      {/* Line 1 — identity and headline numbers, always one line. */}
      <div className="flex items-center gap-2 text-sm">
        {hasFailures ? (
          <button
            type="button"
            className="text-fact hover:text-foreground shrink-0"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-label={`Toggle failures for ${run.pipeline_id ?? shortSHA}`}
            data-testid="run-row-toggle"
          >
            {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </button>
        ) : (
          // Nothing to expand on a run without failures; the spacer keeps the
          // verdict glyph in the same column as on failing rows.
          <span aria-hidden="true" className="w-4 shrink-0" />
        )}

        <span aria-hidden="true" className={cn('shrink-0', STATUS_TEXT_CLASSES[verdict])}>
          {glyph}
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
          // The branch is how a developer finds their run, so it takes the full
          // ink colour rather than the metadata gray around it.
          <span className="text-foreground inline-flex shrink-0 items-center gap-1 text-xs">
            <GitBranch size={11} />
            {run.branch}
          </span>
        )}

        {groupLabel &&
          (groupHref ? (
            <NavLink
              to={groupHref}
              className="text-fact min-w-0 truncate text-xs hover:underline"
            >
              {groupLabel}
            </NavLink>
          ) : (
            <span className="text-fact min-w-0 truncate text-xs">{groupLabel}</span>
          ))}

        <span className="text-fact ml-auto flex shrink-0 items-center gap-3 text-xs">
          {/* A plain fact, not a verdict: coloured by threshold, a failing run at
              95.9% read green. The glyph and the failed-test count carry status. */}
          <span>{passRate}</span>
          <span data-testid="run-row-summary">
            {/* Spell out "failing" — a bare "7/8 suites" reads as 7 passing. */}
            {isFailing ? (
              <>
                {`${suitesFailing}/${aggregate.suites_total} suites failing`}
                {hasFailures && (
                  <>
                    {' · '}
                    <span className={cn('font-medium', STATUS_TEXT_CLASSES.failed)}>
                      {`${failedCount} ${failedCount === 1 ? 'failed test' : 'failed tests'}`}
                    </span>
                  </>
                )}
              </>
            ) : allSkipped ? (
              'all tests skipped'
            ) : (
              `${aggregate.suites_total}/${aggregate.suites_total} suites passed`
            )}
          </span>
          <span className="tabular-nums">{formatDuration(aggregate.total_duration_ms)}</span>
          <span className="tabular-nums">{formatDate(run.timestamp)}</span>
        </span>
      </div>

      {/* Line 2 — only when something failed, so a green run stays one line. */}
      {hasFailures && (
        <div className="mt-1.5 pl-6">
          <RunSuiteChips suites={run.suites} />
        </div>
      )}

      {expanded && hasFailures && (
        <div className="mt-2 pl-6">
          <RunFailures run={run} />
        </div>
      )}
    </div>
  )
}
