import { useEffect, useReducer, useState } from 'react'
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
import { formatRelativeTime, splitBranch } from './rowFormat'
import type { PipelineRun } from '@/types/api'

export interface RunRowProps {
  run: PipelineRun
}

// The label is a function of the clock and React Query will not re-render an unchanged
// feed, so a page left open ticks on its own. ponytail: one timer per row, lift into
// the feed page if pages ever hold hundreds of rows.
function RelativeTime({ timestamp, className }: { timestamp: string; className: string }) {
  const [, tick] = useReducer((n: number) => n + 1, 0)
  useEffect(() => {
    const id = setInterval(tick, 60_000)
    return () => clearInterval(id)
  }, [])
  return (
    <time dateTime={timestamp} title={formatDate(timestamp)} className={className}>
      {formatRelativeTime(timestamp)}
    </time>
  )
}

export function RunRow({ run }: RunRowProps) {
  const { aggregate } = run
  // Collapsed by default. Auto-expanding every failing run turned a page of
  // ten runs into an unscannable wall, and it fetched failures for all of them
  // before the user had asked for any.
  const [expanded, setExpanded] = useState(false)

  const shortSHA = run.commit_sha?.slice(0, 7)
  const [branchHead, branchTail] = splitBranch(run.branch ?? '')
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
      {/* Line 1 — identity and headline numbers. A grid of fixed columns when the feed is
          wide enough (a container query, because the room depends on the sidebar), so the
          figures line up from row to row; below that it stacks into three short lines
          (the two wrappers become the second and third). */}
      <div className="grid grid-cols-[1rem_1rem_minmax(0,1fr)_auto] items-baseline gap-x-2 gap-y-1 text-sm @4xl:grid-cols-[1rem_1rem_minmax(5rem,max-content)_minmax(0,1fr)_8rem_minmax(14rem,max-content)_3rem_6rem] @5xl:grid-cols-[1rem_1rem_minmax(5rem,max-content)_minmax(0,1fr)_8rem_minmax(14rem,max-content)_3rem_3.5rem_6rem]">
        {hasFailures ? (
          <button
            type="button"
            className="text-fact hover:text-foreground self-center"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-label={`Toggle failures for ${run.pipeline_id ?? shortSHA}`}
            data-testid="run-row-toggle"
          >
            {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </button>
        ) : (
          // Nothing to expand on a run without failures; the empty cell keeps the
          // verdict glyph in the same column as on failing rows.
          <span aria-hidden="true" />
        )}

        <span aria-hidden="true" className={cn('text-center', STATUS_TEXT_CLASSES[verdict])}>
          {glyph}
        </span>

        <code className="text-fact text-xs font-semibold">
          {(run.pipeline_url ?? run.ci_build_url) ? (
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

        <div className="col-start-3 col-end-5 row-start-2 flex min-w-0 items-baseline gap-2 @4xl:contents">
          {/* The branch is how a developer finds their run: full ink, 14px, weight 500.
              The cell stays when a run has no branch so the columns do not shift. */}
          <span
            title={run.branch}
            className="text-foreground flex min-w-0 flex-1 items-center font-medium"
          >
            {run.branch && (
              <>
                <GitBranch size={11} className="mr-1 shrink-0" />
                {/* Inline boxes, not flex items: a copied branch stays one line, and a
                    screen reader reads it once. flex-1 gives the percentage a definite
                    width; 9ch leaves room for the 8-character tail. */}
                <span className="min-w-0 flex-1 overflow-hidden whitespace-nowrap">
                  <span
                    className={cn(
                      'inline-block truncate align-bottom',
                      branchTail ? 'max-w-[calc(100%-9ch)]' : 'max-w-full',
                    )}
                  >
                    {branchHead}
                  </span>
                  {branchTail}
                </span>
              </>
            )}
          </span>

          <span className="text-fact max-w-[40%] min-w-0 shrink-0 text-xs @4xl:max-w-none">
            {groupLabel &&
              (groupHref ? (
                // truncate on the link, not a wrapper: a wrapper would clip the focus ring.
                <NavLink to={groupHref} className="block truncate hover:underline">
                  {groupLabel}
                </NavLink>
              ) : (
                <span className="block truncate">{groupLabel}</span>
              ))}
          </span>
        </div>

        <div className="text-fact col-start-3 col-end-5 row-start-3 flex min-w-0 items-baseline justify-between gap-3 text-xs @4xl:contents">
          <span data-testid="run-row-summary" className="min-w-0 @4xl:truncate">
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
          {/* A plain fact, not a verdict: coloured by threshold, a failing run at
              95.9% read green. The glyph and the failed-test count carry status. */}
          <span className="text-right tabular-nums">{passRate}</span>
          <span className="hidden text-right tabular-nums @5xl:block">
            {formatDuration(aggregate.total_duration_ms)}
          </span>
        </div>

        <RelativeTime
          timestamp={run.timestamp}
          className="text-fact col-start-4 row-start-1 text-right text-xs whitespace-nowrap tabular-nums @4xl:col-start-auto @4xl:row-start-auto"
        />
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
