import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, NavLink, useLocation, useParams } from 'react-router'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import {
  AlertCircle,
  ChevronRight,
  ExternalLink,
  GitBranch,
  GitCommitHorizontal,
  RefreshCw,
} from 'lucide-react'

import { ApiError, extractErrorMessage } from '@/api/client'
import { env } from '@/lib/env'
import { useDocumentTitle } from '@/lib/useDocumentTitle'
import { pipelineRunOptions, runFailuresOptions } from '@/lib/queries'
import { cn, formatDuration, formatPassRate } from '@/lib/utils'
import { useUIStore, type FailureGrouping } from '@/store/ui'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { RelativeTime, RunGlyph, RunGroupLink, RunSummary } from './RunFacts'
import { RunFailures } from './RunFailures'
import { countOrigins, originParts } from './failureOrigin'
import { mergeRunFailures } from './mergeRunFailures'
import { canSayNoneNew, failingBuildIds, isFailingSuite, summarizeRun } from './rowFormat'
import { runIdOf, runKeyOf, suiteReportHref } from './runKey'
import type { PipelineRun } from '@/types/api'

/**
 * One pipeline run on its own page, `/runs/:groupId/:runKey`: where CI links, MCP
 * and feed rows land. Failing suites lead, one open section each; the rest collapse
 * into one line.
 */
export function RunPage() {
  const { groupId = '', runKey = '' } = useParams()
  const { data, isLoading, isFetching, error, refetch } = useQuery(
    pipelineRunOptions(groupId, runKey),
  )
  const run = data?.data

  useDocumentTitle(
    run ? `${runIdOf(run)} · ${run.branch || 'no branch'} — ${env.appTitle}` : undefined,
  )

  if (isLoading) {
    // The header's real height, so the page does not jump when the run arrives.
    return (
      <div className="space-y-6" data-testid="run-page">
        <div className="space-y-1.5">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-5 w-72 max-w-full" />
          <Skeleton className="h-4 w-96 max-w-full" />
          <Skeleton className="h-5 w-80 max-w-full" />
        </div>
        <Skeleton className="h-40 w-full rounded-lg" />
      </div>
    )
  }

  // Keyed by run: an opened list or a deep-link view must not carry over to the next
  // run, which shows without a loading state in between when it is already cached.
  if (run) return <RunView key={`${groupId}/${runKey}`} run={run} />

  if (error instanceof ApiError && error.response?.status === 404) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed px-4 py-16 text-center">
        <GitCommitHorizontal size={36} className="text-muted-foreground/40" aria-hidden="true" />
        <div>
          <h1 className="font-medium">Run not found</h1>
          <p className="text-fact text-sm wrap-anywhere">
            This group has no run <code className="font-mono">{runKey}</code>.
          </p>
        </div>
        <Link to="/" className="text-primary text-sm hover:underline">
          Back to Runs
        </Link>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center gap-2 py-16 text-center">
      <AlertCircle className="text-destructive h-5 w-5" aria-hidden="true" />
      <h1 className="text-destructive text-sm font-medium">Couldn&apos;t load this run</h1>
      <p className="text-fact max-w-xs text-xs">{extractErrorMessage(error)}</p>
      <Button
        variant="outline"
        size="sm"
        className="mt-1 h-7 gap-1 text-xs"
        disabled={isFetching}
        onClick={() => void refetch()}
      >
        <RefreshCw className={cn('h-3 w-3', isFetching && 'animate-spin')} />
        Retry
      </Button>
    </div>
  )
}

function RunView({ run }: { run: PipelineRun }) {
  const location = useLocation()
  const { suitesFailing } = summarizeRun(run)
  const isFailing = suitesFailing > 0
  const ciUrl = run.pipeline_url ?? run.ci_build_url
  const { aggregate } = run

  const others = run.suites.filter((s) => !isFailingSuite(s))
  const skippedCount = others.filter((s) => s.status === 'skipped').length
  const passedCount = others.length - skippedCount
  const suites = (n: number) => `${n} ${n === 1 ? 'suite' : 'suites'}`
  const othersLabel =
    skippedCount === 0
      ? `${suites(passedCount)} passed`
      : passedCount === 0
        ? `${suites(skippedCount)} skipped`
        : `${suites(passedCount)} passed · ${skippedCount} skipped`

  // #suite-<id> deep links (feed chips, CI, MCP) must land on something visible.
  const targetId = location.hash.slice(1)
  const targetsOther = others.some((s) => `suite-${s.project_id}` === targetId)
  const targetsFailing = run.suites.some(
    (s) => isFailingSuite(s) && `suite-${s.project_id}` === targetId,
  )

  // A suite link to a failing suite asks for the suite view ("by error" has no suite
  // sections): shown for this visit only, the saved preference is left alone.
  const savedGrouping = useUIStore((s) => s.runsFailureGrouping)
  const wantsSuites = targetsFailing && savedGrouping === 'error'
  const [groupingOverride, setGroupingOverride] = useState<FailureGrouping | undefined>(
    wantsSuites ? 'suite' : undefined,
  )

  // A run with nothing failing shows its suites open: they are all there is.
  const [othersOpen, setOthersOpen] = useState(!isFailing || targetsOther)
  const [seenKey, setSeenKey] = useState(location.key)
  if (seenKey !== location.key) {
    setSeenKey(location.key)
    if (targetsOther) setOthersOpen(true)
    if (wantsSuites) setGroupingOverride('suite')
  }

  // The failing sections sit above everything else and exist only once the failures
  // settle, so no target is scrolled to before then: they would push it away.
  // dataUpdatedAt also moves on a success after an error (a Retry).
  const failuresQuery = useQuery({
    ...runFailuresOptions(run.group_project_id ?? 0, runKeyOf(run), failingBuildIds(run)),
    // The same as in RunFailures, so the header's origin counts do not blink out either.
    placeholderData: keepPreviousData,
    enabled: isFailing && run.group_project_id != null,
  })
  const failuresSettled =
    !isFailing || run.group_project_id == null || failuresQuery.status !== 'pending'
  const handledKey = useRef<string | null>(null)
  useEffect(() => {
    if (!targetId || !failuresSettled || handledKey.current === location.key) return
    const target = document.getElementById(targetId)
    if (!target) return
    handledKey.current = location.key
    target.scrollIntoView({ block: 'start' })
    // The section's heading, or the passed-suite row itself.
    const focusable = target.querySelector<HTMLElement>('[tabindex="-1"]') ?? target
    focusable.focus({ preventScroll: true })
  }, [
    location.key,
    targetId,
    failuresSettled,
    failuresQuery.dataUpdatedAt,
    groupingOverride,
    othersOpen,
  ])

  // "3 new · 2 inherited (1 env)": from the query RunFailures already made, once it
  // has loaded; counted over the same merged rows the panel lists.
  const failuresData = failuresQuery.data
  const origins = useMemo(() => {
    if (!failuresData) return []
    const listed = mergeRunFailures(failuresData.data)
    return originParts(
      countOrigins(listed),
      failuresData.metadata.truncated ? listed.length : undefined,
    ).filter((part) => part !== 'none new' || canSayNoneNew(run))
  }, [failuresData, run])

  const passRate = formatPassRate(
    aggregate.tests_passed,
    aggregate.tests_total,
    aggregate.tests_skipped,
  )

  return (
    <div className="space-y-6" data-testid="run-page">
      <header className="space-y-1.5">
        <h1 className="flex items-baseline gap-2 text-2xl font-semibold">
          <RunGlyph run={run} />
          {/* Machine-Text Rule: the ID is what a developer pastes into a terminal. */}
          <span className="min-w-0 font-mono wrap-anywhere">{runIdOf(run)}</span>
        </h1>

        {/* The whole branch, wrapping if it must: no ellipsis on the page that names it. */}
        <p className="text-foreground flex items-baseline gap-1.5 text-sm font-medium">
          <GitBranch size={12} className="shrink-0 self-center" aria-hidden="true" />
          {run.branch ? (
            <span className="min-w-0 wrap-anywhere">{run.branch}</span>
          ) : (
            <span className="text-fact font-normal">no branch</span>
          )}
        </p>

        <p className="text-fact flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs">
          {run.pipeline_id && <code className="font-mono">{run.commit_sha.slice(0, 7)}</code>}
          <RunGroupLink run={run} className="min-w-0 wrap-anywhere" />
          {ciUrl && (
            <a
              href={ciUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label="CI pipeline (opens in new tab)"
              className="inline-flex items-center gap-1 hover:underline"
            >
              CI pipeline
              <ExternalLink size={11} aria-hidden="true" />
            </a>
          )}
          <RelativeTime timestamp={run.timestamp} className="tabular-nums" />
        </p>

        <p className="text-fact text-sm" data-testid="run-page-summary">
          <RunSummary
            run={run}
            extra={[
              ...origins,
              // "—" means nothing could be rated: no "— passed".
              passRate !== '—' && `${passRate} passed`,
              formatDuration(aggregate.total_duration_ms),
            ]}
          />
        </p>
      </header>

      {isFailing && (
        <RunFailures
          run={run}
          asPage
          grouping={groupingOverride}
          onGroupingChange={() => setGroupingOverride(undefined)}
        />
      )}

      {others.length > 0 && (
        // Collapse-the-Good: suites that did not fail fold into one quiet line.
        <details
          open={othersOpen}
          onToggle={(e) => setOthersOpen(e.currentTarget.open)}
          className="group"
        >
          <summary className="text-fact hover:text-foreground inline-flex cursor-pointer list-none items-center gap-1.5 text-sm [&::-webkit-details-marker]:hidden">
            <ChevronRight
              size={14}
              className="shrink-0 transition-transform group-open:rotate-90"
              aria-hidden="true"
            />
            {othersLabel}
          </summary>
          <ul className="mt-2 divide-y rounded-lg border">
            {others.map((s) => (
              <li
                key={s.project_id}
                id={`suite-${s.project_id}`}
                tabIndex={-1}
                className="focus:ring-ring grid scroll-mt-6 grid-cols-[minmax(0,1fr)_auto_auto] items-baseline gap-3 px-3 py-1.5 text-sm focus:ring-2 focus:outline-none focus:ring-inset"
              >
                <NavLink to={suiteReportHref(s)} className="min-w-0 truncate hover:underline">
                  {s.display_name || s.slug}
                </NavLink>
                <span className="text-fact text-right text-xs tabular-nums">
                  {formatPassRate(
                    s.passed ?? s.total - s.failed - (s.skipped ?? 0),
                    s.total,
                    s.skipped,
                  )}
                </span>
                <span className="text-fact w-14 text-right text-xs tabular-nums">
                  {formatDuration(s.duration_ms)}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}
