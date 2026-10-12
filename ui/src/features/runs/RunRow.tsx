import { useId, useState } from 'react'
import { Link } from 'react-router'
import { ChevronDown, ChevronRight, ExternalLink, GitBranch } from 'lucide-react'

import { formatDuration, formatPassRate } from '@/lib/utils'
import { cn } from '@/lib/utils'
import { RunSuiteChips } from './RunSuiteChips'
import { RunFailures } from './RunFailures'
import { RelativeTime, RunGlyph, RunGroupLink, RunSummary } from './RunFacts'
import { originParts } from './failureOrigin'
import { splitBranch, summarizeRun } from './rowFormat'
import { runIdOf, runPath } from './runKey'
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
  const panelId = useId()

  const runId = runIdOf(run)
  const href = runPath(run)
  const [branchHead, branchTail] = splitBranch(run.branch ?? '')
  // A suite whose report could not be read fails the run with no test listed as
  // failed, so "failing" and "has failures to expand" are different questions.
  const { failedCount } = summarizeRun(run)
  const hasFailures = failedCount > 0
  // "2 new · 3 inherited (1 env)", as the expanded panel's header says it. On the chips
  // line, not the summary: a longer summary would widen its column and squeeze the branch.
  const counts = aggregate.origins
  const origins = counts
    ? originParts({
        new: counts.new,
        inherited: counts.inherited,
        env: counts.environment,
        unknown: 0,
      })
    : []
  const passRate = formatPassRate(
    aggregate.tests_passed,
    aggregate.tests_total,
    aggregate.tests_skipped,
  )

  // The branch is how a developer finds their run (full ink, 14px, weight 500), and it
  // opens the run's page. The cell stays when a run has no branch so the columns hold.
  const branchClass = 'text-foreground flex min-w-0 flex-1 items-center font-medium'
  const branchContent = run.branch ? (
    <>
      <GitBranch size={11} className="mr-1 shrink-0" />
      {/* Inline boxes, not flex items: a copied branch stays one line, and a screen
          reader reads it once. flex-1 gives the percentage a definite width; 9ch
          leaves room for the 8-character tail. */}
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
  ) : (
    // Also without a link: the cell is the row's heading, which must not be empty.
    <span className="text-fact font-normal">no branch</span>
  )

  return (
    <li className="px-4 py-2.5" data-testid="run-row">
      {/* Line 1 — identity and headline numbers. A grid of fixed columns when the feed is
          wide enough (a container query, because the room depends on the sidebar), so the
          figures line up from row to row; below that it stacks into three short lines
          (the two wrappers become the second and third). */}
      <div className="grid grid-cols-[1rem_1rem_minmax(0,1fr)_auto] items-baseline gap-x-2 gap-y-1 text-sm @4xl:grid-cols-[1rem_1rem_minmax(5rem,max-content)_minmax(0,1fr)_8rem_minmax(14rem,max-content)_3rem_6rem] @5xl:grid-cols-[1rem_1rem_minmax(5rem,max-content)_minmax(0,1fr)_8rem_minmax(14rem,max-content)_3rem_3.5rem_6rem]">
        {hasFailures ? (
          <button
            type="button"
            // -m-1 p-1: a 24px target that still takes the 16px column.
            className="text-fact hover:text-foreground -m-1 self-center p-1"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-controls={expanded ? panelId : undefined}
            aria-label={`Toggle failures for ${runId}`}
            data-testid="run-row-toggle"
            // j/k stop here when the row has no branch link.
            data-row-link={href ? undefined : true}
          >
            {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </button>
        ) : (
          // Nothing to expand on a run without failures; the empty cell keeps the
          // verdict glyph in the same column as on failing rows.
          <span aria-hidden="true" />
        )}

        <RunGlyph run={run} className="text-center" />

        <code className="text-fact text-xs font-semibold">
          {(run.pipeline_url ?? run.ci_build_url) ? (
            <a
              href={run.pipeline_url ?? run.ci_build_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 hover:underline"
            >
              {runId}
              <ExternalLink size={11} />
            </a>
          ) : (
            runId
          )}
        </code>

        <div className="col-start-3 col-end-5 row-start-2 flex min-w-0 items-baseline gap-2 @4xl:contents">
          {/* Each run's heading, for jumping run to run; contents keeps the cell in the grid. */}
          <h2 className="contents">
            {href ? (
              <Link
                to={href}
                title={run.branch || undefined}
                // Every row has a "main" link; the run ID says which run this one opens.
                aria-label={`${run.branch || 'no branch'}, run ${runId}`}
                className={cn(branchClass, 'hover:underline')}
                data-row-link
              >
                {branchContent}
              </Link>
            ) : (
              <span title={run.branch || undefined} className={branchClass}>
                {branchContent}
              </span>
            )}
          </h2>

          {/* min-w: a narrow feed keeps a few letters of the group, never a bare ellipsis. */}
          <span className="text-fact max-w-[40%] min-w-[6ch] shrink-0 text-xs @4xl:max-w-none">
            {/* truncate on the link, not a wrapper: a wrapper would clip the focus ring. */}
            <RunGroupLink run={run} className="block truncate" />
          </span>
        </div>

        <div className="text-fact col-start-3 col-end-5 row-start-3 flex min-w-0 items-baseline justify-between gap-3 text-xs @4xl:contents">
          <span data-testid="run-row-summary" className="min-w-0 @4xl:truncate">
            <RunSummary run={run} />
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
          <RunSuiteChips suites={run.suites} runPath={href} origins={origins} />
        </div>
      )}

      {expanded && hasFailures && (
        <div id={panelId} className="mt-2 pl-6">
          <RunFailures run={run} />
        </div>
      )}
    </li>
  )
}
