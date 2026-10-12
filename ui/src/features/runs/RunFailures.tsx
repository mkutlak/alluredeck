import { useId, useMemo, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { NavLink, useLocation } from 'react-router'
import { ChevronDown, ChevronRight, FileText, RefreshCw, Sparkles } from 'lucide-react'

import { runFailuresOptions } from '@/lib/queries'
import { getConfig } from '@/api/system'
import { useUIStore, type FailureGrouping } from '@/store/ui'
import { Button } from '@/components/ui/button'
import { Segmented } from '@/components/ui/segmented'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import { FailureBadges } from './FailureBadges'
import { countOrigins, failureBucket, originParts } from './failureOrigin'
import { FailureSummaryPanel } from './FailureSummaryPanel'
import {
  groupByError,
  groupBySuite,
  suiteSections,
  suiteVerdict,
  type FailureGroup,
} from './groupFailures'
import { mergeRunFailures } from './mergeRunFailures'
import { failingBuildIds } from './rowFormat'
import { runKeyOf } from './runKey'
import type { MergedFailure } from './mergeRunFailures'
import type { PipelineRun } from '@/types/api'

export interface RunFailuresProps {
  run: PipelineRun
  /**
   * The run page: every failing suite gets an open, anchored `<section id="suite-<id>">`,
   * reopened when the URL hash points at it.
   */
  asPage?: boolean
  /** Shown instead of the saved grouping (the run page's deep-link view); the store is untouched. */
  grouping?: FailureGrouping
  /** Told when the user picks a grouping; the choice is saved either way. */
  onGroupingChange?: (grouping: FailureGrouping) => void
}

/** Rows shown per group before the "… N more" expander appears. */
const ROWS_PER_GROUP = 8

const GROUPING_OPTIONS = [
  { value: 'suite' as const, label: 'by suite', 'data-testid': 'run-failure-grouping-suite' },
  { value: 'error' as const, label: 'by error', 'data-testid': 'run-failure-grouping-error' },
]

// One grid template per group, which every row of the group shares as a subgrid (see
// FailureRow): the Locked-Column Rule. A template per row sized its auto badge column per
// row, so the error text started at a different x on every row.
// At @xl: by suite, name · badges · error; by error, name · suite · badges. The AI control
// takes a last column only when the feature is on. The name track counts the row's 2.25rem
// indent, which in a subgrid sits inside the first track (32ch of name, as before: the
// group carries text-sm, so ch is measured at 14px as it was on the row).
// Below 36rem of panel the error column had no room left (0px at a 390px viewport), so
// there the error drops under the test name and spans the row (see ErrorText). The test
// name and the badges span it too, and by suite the AI control goes on a line of its own
// after the error, so it comes where the DOM order puts it: beside the name, the badges
// and the AI label left it a few characters.
// By error, the suite name likewise moves under the test name instead of taking 10rem of it.
const GRID_BY_SUITE = '@xl:grid-cols-[minmax(14.25rem,calc(32ch+2.25rem))_auto_minmax(0,1fr)]'
const GRID_BY_SUITE_AI =
  '@xl:grid-cols-[minmax(14.25rem,calc(32ch+2.25rem))_auto_minmax(0,1fr)_auto]'
const GRID_BY_ERROR = '@xl:grid-cols-[minmax(0,1fr)_10rem_auto]'
const GRID_BY_ERROR_AI = '@xl:grid-cols-[minmax(0,1fr)_10rem_auto_auto]'

export function RunFailures({
  run,
  asPage = false,
  grouping: groupingOverride,
  onGroupingChange,
}: RunFailuresProps) {
  const groupProjectId = run.group_project_id
  const runKey = runKeyOf(run)

  const savedGrouping = useUIStore((s) => s.runsFailureGrouping)
  const setGrouping = useUIStore((s) => s.setRunsFailureGrouping)
  const grouping = groupingOverride ?? savedGrouping

  // Shares the app-wide ['config'] cache entry, so the AI-summary toggle can be
  // hidden entirely when the feature is off instead of expanding to nothing.
  const { data: configResp } = useQuery({ queryKey: ['config'], queryFn: getConfig })
  const llmEnabled = configResp?.data.llm_enabled === true

  // This component is only mounted once its run is expanded, so a feed of
  // collapsed runs issues no failure requests at all. The key changes when a failing suite
  // arrives (see runFailuresOptions): the list on screen stays until the new one is in.
  const { data, isLoading, isError, isFetching, refetch } = useQuery({
    ...runFailuresOptions(groupProjectId ?? 0, runKey, failingBuildIds(run)),
    placeholderData: keepPreviousData,
    enabled: groupProjectId != null && runKey !== '',
  })

  const failures = useMemo(() => mergeRunFailures(data?.data ?? []), [data])
  // With no rows there is nothing to group by error, but the page's failing suites
  // still need their sections (and the toggle), so it falls back to suites.
  const bySuite = grouping === 'suite' || (asPage && failures.length === 0)
  const groups = useMemo(
    () =>
      !bySuite
        ? groupByError(failures)
        : asPage
          ? suiteSections(failures, run.suites)
          : groupBySuite(failures, run.suites),
    [failures, bySuite, run.suites, asPage],
  )

  const [openOverrides, setOpenOverrides] = useState<Record<string, boolean>>({})
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({})
  const [openSummaries, setOpenSummaries] = useState<Record<string, boolean>>({})

  // Each navigation to #suite-<id> reopens that section if the user had collapsed it,
  // so a deep link never lands on a closed group.
  const location = useLocation()
  const [hashKey, setHashKey] = useState<string | null>(null)
  if (asPage && hashKey !== location.key) {
    setHashKey(location.key)
    const target = location.hash.slice(1)
    if (openOverrides[target] === false) setOpenOverrides((prev) => ({ ...prev, [target]: true }))
  }

  if (groupProjectId == null) {
    return (
      <p className="text-fact text-sm" data-testid="run-failures">
        Failure details are unavailable for this run.
      </p>
    )
  }

  if (isLoading) {
    return (
      <div className="space-y-2" data-testid="run-failures">
        <Skeleton className="h-7 w-40" />
        <Skeleton className="h-5 w-full" />
        <Skeleton className="h-5 w-3/4" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center gap-3" data-testid="run-failures">
        <p className="text-destructive text-sm">Failed to load failures for this run.</p>
        <Button
          variant="outline"
          size="sm"
          className="h-7 gap-1 text-xs"
          disabled={isFetching}
          onClick={() => void refetch()}
        >
          <RefreshCw className={cn('h-3 w-3', isFetching && 'animate-spin')} />
          Retry
        </Button>
      </div>
    )
  }

  if (groups.length === 0) {
    return (
      <p className="text-fact text-sm" data-testid="run-failures">
        No failing tests in this run.
      </p>
    )
  }

  // In the feed the first group opens by default so expanding a run shows something
  // useful immediately, without unfolding every group at once. The page opens them all.
  const isGroupOpen = (key: string, index: number) => openOverrides[key] ?? (asPage || index === 0)
  const toggleGroup = (key: string, index: number) =>
    setOpenOverrides((prev) => ({ ...prev, [key]: !(prev[key] ?? (asPage || index === 0)) }))

  const truncated = data?.metadata.truncated === true
  // Only where the failures came from: the collapsed row and its chips already say how many
  // tests failed in how many suites. Unknown origins are counted too, so "2 new" never
  // reads as the whole answer when it is not.
  const origins = originParts(countOrigins(failures), truncated ? failures.length : undefined)

  return (
    <div
      className={cn('@container border', asPage ? 'rounded-lg' : 'rounded-md')}
      data-testid="run-failures"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
        <div data-testid="run-failure-grouping" className="shrink-0">
          <Segmented<FailureGrouping>
            value={grouping}
            onValueChange={(next) => {
              onGroupingChange?.(next)
              setGrouping(next)
            }}
            options={GROUPING_OPTIONS}
            size="xs"
            aria-label="Group failures by"
          />
        </div>
        {origins.length > 0 && (
          <span className="text-fact text-xs" data-testid="run-failure-origins">
            {origins.join(' · ')}
          </span>
        )}
      </div>

      <div className="divide-y">
        {groups.map((group, index) => (
          <FailureGroupBlock
            key={group.key}
            group={group}
            grouping={grouping}
            open={isGroupOpen(group.key, index)}
            onToggle={() => toggleGroup(group.key, index)}
            allRowsShown={expandedGroups[group.key] ?? false}
            onShowAllRows={() => setExpandedGroups((prev) => ({ ...prev, [group.key]: true }))}
            llmEnabled={llmEnabled}
            asSection={asPage && bySuite}
            // ✗, or ⚠ "flaky or known" (suiteVerdict); the run's own glyph is ✗,
            // or ○ while suites are awaited.
            sectionVerdict={asPage && bySuite ? suiteVerdict(group, truncated) : undefined}
            historyBranch={run.branch}
            emptyNote={
              truncated
                ? 'Not listed: this run has more failures than were loaded.'
                : 'No failing tests listed for this suite.'
            }
            openSummaries={openSummaries}
            onToggleSummary={(key) => setOpenSummaries((prev) => ({ ...prev, [key]: !prev[key] }))}
          />
        ))}
      </div>

      {data?.metadata.truncated && (
        <p className="text-fact border-t px-3 py-2 text-xs">
          Showing the first {failures.length} failures; this run has more.
        </p>
      )}
    </div>
  )
}

interface FailureGroupBlockProps {
  group: FailureGroup
  grouping: FailureGrouping
  open: boolean
  onToggle: () => void
  allRowsShown: boolean
  onShowAllRows: () => void
  llmEnabled: boolean
  /** Page mode: an anchored section with an h2 the hash can focus. */
  asSection: boolean
  /** Test names link to their history on this branch; '' for none (a run without a branch). */
  historyBranch: string
  /** Shown in place of rows for a failing suite none of whose rows arrived. */
  emptyNote: string
  /** Page sections: ✗, or ⚠ "flaky or known" when every listed failure is one. */
  sectionVerdict?: 'failed' | 'tolerated'
  openSummaries: Record<string, boolean>
  onToggleSummary: (key: string) => void
}

function FailureGroupBlock({
  group,
  grouping,
  open,
  onToggle,
  allRowsShown,
  onShowAllRows,
  llmEnabled,
  asSection,
  historyBranch,
  emptyNote,
  sectionVerdict,
  openSummaries,
  onToggleSummary,
}: FailureGroupBlockProps) {
  const rows = allRowsShown ? group.rows : group.rows.slice(0, ROWS_PER_GROUP)
  const hiddenCount = group.rows.length - rows.length
  const headingId = `${group.key}-heading`

  const toggle = (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      className="text-fact hover:text-foreground min-w-[min(100%,12rem)] flex-1 text-left"
    >
      <span className="flex items-center gap-2">
        {open ? (
          <ChevronDown size={14} className="shrink-0" />
        ) : (
          <ChevronRight size={14} className="shrink-0" />
        )}
        {sectionVerdict && (
          <span
            aria-hidden="true"
            className={cn(
              'shrink-0',
              STATUS_TEXT_CLASSES[sectionVerdict === 'failed' ? 'failed' : 'broken'],
            )}
          >
            {sectionVerdict === 'failed' ? '✗' : '⚠'}
          </span>
        )}
        <span
          className="text-foreground min-w-0 truncate text-sm font-medium"
          title={group.message}
        >
          {group.message ?? group.title}
        </span>
        {/* The words, so neither the glyph nor its colour carries this alone. */}
        {/* The spaces keep accessible names like "api-cloud flaky or known"; flex hides them. */}
        {sectionVerdict === 'tolerated' && (
          <>
            {' '}
            <span className="text-fact shrink-0 text-xs">flaky or known</span>
          </>
        )}
        {group.subtitle && (
          <>
            {' '}
            <span className="text-fact shrink-0 text-xs">{group.subtitle}</span>
          </>
        )}
      </span>
      {/* Clusters sharing a message differ only in this step, so it is shown whole on its own
          line, indented to the rows below. */}
      {group.step && (
        <>
          {' '}
          <span className="text-fact block pl-6 text-xs wrap-anywhere">{`in ${group.step}`}</span>
        </>
      )}
    </button>
  )

  const content = (
    <>
      {/* Wraps: eight shards with four-digit build numbers outgrow a phone's row. The
          title keeps 12rem before the build links move to a line of their own. */}
      <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 px-3 py-1.5">
        {asSection ? (
          // The heading wraps the toggle (the accordion pattern); tabIndex -1 lets a
          // #suite- link move focus here, and the ring shows where the link landed.
          <h2
            id={headingId}
            tabIndex={-1}
            className="focus:ring-ring flex min-w-[min(100%,12rem)] flex-1 rounded-sm focus:ring-2 focus:outline-none"
          >
            {toggle}
          </h2>
        ) : (
          toggle
        )}

        {/* Every contributing build stays reachable, so a sharded suite does
            not hide two thirds of its reports behind one link. */}
        {group.suite?.builds && (
          <span className="ml-auto flex flex-wrap justify-end gap-x-2">
            {group.suite.builds.map((build) => (
              <NavLink
                key={build.build_id}
                to={`/projects/${group.suite?.project_id ?? 0}/reports/${build.build_number}`}
                className="text-fact hover:text-foreground inline-flex shrink-0 items-center gap-0.5 text-xs hover:underline"
              >
                {`#${build.build_number}`}
                <FileText size={10} />
              </NavLink>
            ))}
          </span>
        )}
      </div>

      {open && (
        <div className="pb-1.5">
          {rows.length === 0 && <p className="text-fact px-3 py-1 pl-9 text-xs">{emptyNote}</p>}
          <div
            className={cn(
              'grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 text-sm',
              grouping === 'suite'
                ? llmEnabled
                  ? GRID_BY_SUITE_AI
                  : GRID_BY_SUITE
                : llmEnabled
                  ? GRID_BY_ERROR_AI
                  : GRID_BY_ERROR,
            )}
          >
            {rows.map((row) => (
              <FailureRow
                key={row.key}
                row={row}
                grouping={grouping}
                historyBranch={historyBranch}
                llmEnabled={llmEnabled}
                summaryOpen={openSummaries[row.key] ?? false}
                onToggleSummary={() => onToggleSummary(row.key)}
              />
            ))}
          </div>

          {hiddenCount > 0 && (
            <button
              type="button"
              onClick={onShowAllRows}
              className="text-fact hover:text-foreground px-3 py-1 pl-9 text-xs underline-offset-2 hover:underline"
            >
              {`… ${hiddenCount} more`}
            </button>
          )}
        </div>
      )}
    </>
  )

  return asSection ? (
    <section
      id={group.key}
      aria-labelledby={headingId}
      className="scroll-mt-6"
      data-testid="run-failure-group"
    >
      {content}
    </section>
  ) : (
    <div data-testid="run-failure-group">{content}</div>
  )
}

interface FailureRowProps {
  row: MergedFailure
  grouping: FailureGrouping
  historyBranch: string
  llmEnabled: boolean
  summaryOpen: boolean
  onToggleSummary: () => void
}

function FailureRow({
  row,
  grouping,
  historyBranch,
  llmEnabled,
  summaryOpen,
  onToggleSummary,
}: FailureRowProps) {
  const reportHref = `/projects/${row.projectId}/reports/${row.buildNumber}`
  const summaryId = useId()
  // The test's history ("is it me or flaky?") comes first, the report second, in the feed
  // and on the run page alike. TestHistoryPage needs a history_id; a row without one keeps
  // its report link.
  const historyHref = row.historyId
    ? `/projects/${row.projectId}/tests?${new URLSearchParams({
        history_id: row.historyId,
        ...(historyBranch ? { branch: historyBranch } : {}),
      }).toString()}`
    : undefined

  const bucket = failureBucket(row)
  // Narrow, the group has a line of its own, where an empty badge list would indent the rest.
  const badges = (
    <div
      className={cn(
        'flex shrink-0 flex-wrap items-center gap-1 @xl:flex-nowrap @max-xl:[&>:empty]:hidden',
        // By suite, narrow: the whole line, so the AI control falls after the error.
        grouping === 'suite' && 'col-span-2 @xl:col-span-1',
      )}
    >
      <FailureBadges
        flaky={row.flaky}
        retries={row.retries}
        newFailed={row.newFailed}
        known={row.known}
        inherited={bucket === 'inherited'}
        unknown={bucket === 'unknown'}
        environment={row.environment === true}
      />
      {historyHref && (
        <NavLink
          to={reportHref}
          aria-label={`Allure report for ${row.testName}`}
          title={`Allure report #${row.buildNumber}`}
          className="text-fact hover:text-foreground shrink-0"
        >
          <FileText size={12} aria-hidden="true" />
        </NavLink>
      )}
    </div>
  )

  return (
    // Both levels are subgrids, so the cells sit on the group's columns (see GRID_BY_SUITE).
    <div data-testid="run-failure-row" className="col-span-full grid grid-cols-subgrid">
      <div
        className={cn(
          'hover:bg-muted/40 col-span-full grid grid-cols-subgrid px-3 py-1 pl-9',
          grouping === 'suite' ? 'items-baseline gap-y-0.5 @xl:gap-y-3' : 'items-center',
        )}
      >
        <NavLink
          to={historyHref ?? reportHref}
          className="col-span-2 min-w-0 truncate hover:underline @xl:col-span-1"
          title={row.fullName || row.testName}
        >
          {row.testName}
        </NavLink>

        {grouping === 'error' ? (
          <span className="text-fact col-span-2 truncate text-xs @xl:col-span-1" title={row.slug}>
            {row.displayName || row.slug}
          </span>
        ) : null}

        {badges}

        {grouping === 'suite' && row.errorMessage ? <ErrorText message={row.errorMessage} /> : null}

        {llmEnabled && (
          <button
            type="button"
            onClick={onToggleSummary}
            aria-expanded={summaryOpen}
            aria-controls={summaryOpen ? summaryId : undefined}
            aria-label={`Toggle AI failure summary for ${row.testName}`}
            // -my-1 py-1: a 24px target that keeps the row's height.
            className="text-fact hover:text-foreground col-start-2 -my-1 inline-flex shrink-0 items-center gap-1 py-1 text-xs whitespace-nowrap @xl:col-start-4"
          >
            <Sparkles size={12} aria-hidden="true" />
            AI summary
          </button>
        )}
      </div>

      {llmEnabled && summaryOpen && (
        <div id={summaryId} className="col-span-full px-3 pl-9">
          <FailureSummaryPanel
            projectId={row.projectId}
            buildId={row.buildId}
            historyId={row.historyId}
            open={summaryOpen}
          />
        </div>
      )}
    </div>
  )
}

/** Past this many characters (or any line break) the 2-line clamp may hide text. */
const CLAMP_HINT_CHARS = 80

/**
 * The failure's error text, clamped to two lines. It stays plain, selectable
 * text; a separate toggle reveals the rest, because the full text used to live
 * only in a `title` tooltip that a keyboard cannot reach. The toggle appears
 * only when the clamp could actually hide something.
 */
function ErrorText({ message }: { message: string }) {
  const [expanded, setExpanded] = useState(false)
  const textId = useId()
  const trimmed = message.trim()
  const canHide = trimmed.includes('\n') || trimmed.length > CLAMP_HINT_CHARS

  return (
    <div className="col-span-2 min-w-0 text-xs @xl:col-span-1">
      <span
        id={textId}
        className={cn(
          'text-fact block break-words',
          expanded ? 'whitespace-pre-wrap' : 'line-clamp-2 whitespace-pre-line',
        )}
      >
        {message}
      </span>
      {canHide && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
          aria-controls={textId}
          className="text-fact hover:text-foreground cursor-pointer underline-offset-2 hover:underline"
        >
          {expanded ? 'Hide full error' : 'Show full error'}
        </button>
      )}
    </div>
  )
}
