import { useId, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { NavLink } from 'react-router'
import { ChevronDown, ChevronRight, FileText, RefreshCw, Sparkles } from 'lucide-react'

import { runFailuresOptions } from '@/lib/queries'
import { getConfig } from '@/api/system'
import { useUIStore, type FailureGrouping } from '@/store/ui'
import { Button } from '@/components/ui/button'
import { Segmented } from '@/components/ui/segmented'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { FailureBadges } from './FailureBadges'
import { FailureSummaryPanel } from './FailureSummaryPanel'
import { groupByError, groupBySuite, type FailureGroup } from './groupFailures'
import { mergeRunFailures } from './mergeRunFailures'
import { runKeyOf } from './runKey'
import type { MergedFailure } from './mergeRunFailures'
import type { PipelineRun } from '@/types/api'

export interface RunFailuresProps {
  run: PipelineRun
}

/** Rows shown per group before the "… N more" expander appears. */
const ROWS_PER_GROUP = 8

const GROUPING_OPTIONS = [
  { value: 'suite' as const, label: 'by suite', 'data-testid': 'run-failure-grouping-suite' },
  { value: 'error' as const, label: 'by error', 'data-testid': 'run-failure-grouping-error' },
]

// Identical grid templates on every row of a mode keep the columns locked. The
// previous flex layout let a column shift by hundreds of pixels depending on
// whether a row happened to carry an error message.
const GRID_BY_SUITE = 'grid grid-cols-[minmax(12rem,32ch)_auto_minmax(0,1fr)] items-baseline gap-3'
const GRID_BY_ERROR = 'grid grid-cols-[minmax(0,1fr)_10rem_auto] items-center gap-3'

export function RunFailures({ run }: RunFailuresProps) {
  const groupProjectId = run.group_project_id
  const runKey = runKeyOf(run)

  const grouping = useUIStore((s) => s.runsFailureGrouping)
  const setGrouping = useUIStore((s) => s.setRunsFailureGrouping)

  // Shares the app-wide ['config'] cache entry, so the AI-summary toggle can be
  // hidden entirely when the feature is off instead of expanding to nothing.
  const { data: configResp } = useQuery({ queryKey: ['config'], queryFn: getConfig })
  const llmEnabled = configResp?.data.llm_enabled === true

  // This component is only mounted once its run is expanded, so a feed of
  // collapsed runs issues no failure requests at all.
  const { data, isLoading, isError, refetch } = useQuery({
    ...runFailuresOptions(groupProjectId ?? 0, runKey),
    enabled: groupProjectId != null && runKey !== '',
  })

  const failures = useMemo(() => mergeRunFailures(data?.data ?? []), [data])
  const groups = useMemo(
    () => (grouping === 'suite' ? groupBySuite(failures, run.suites) : groupByError(failures)),
    [failures, grouping, run.suites],
  )

  const [openOverrides, setOpenOverrides] = useState<Record<string, boolean>>({})
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({})
  const [openSummaries, setOpenSummaries] = useState<Record<string, boolean>>({})

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
          onClick={() => void refetch()}
        >
          <RefreshCw className="h-3 w-3" />
          Retry
        </Button>
      </div>
    )
  }

  if (failures.length === 0) {
    return (
      <p className="text-fact text-sm" data-testid="run-failures">
        No failing tests in this run.
      </p>
    )
  }

  // The first group opens by default so expanding a run shows something useful
  // immediately, without unfolding every group at once.
  const isGroupOpen = (key: string, index: number) => openOverrides[key] ?? index === 0
  const toggleGroup = (key: string, index: number) =>
    setOpenOverrides((prev) => ({ ...prev, [key]: !(prev[key] ?? index === 0) }))

  const suiteCount = new Set(failures.map((f) => f.projectId)).size
  const newCount = failures.filter((f) => f.newFailed).length

  return (
    <div className="rounded-md border" data-testid="run-failures">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
        <div data-testid="run-failure-grouping" className="shrink-0">
          <Segmented<FailureGrouping>
            value={grouping}
            onValueChange={setGrouping}
            options={GROUPING_OPTIONS}
            size="xs"
            aria-label="Group failures by"
          />
        </div>
        <span className="text-fact text-xs">
          {`${failures.length} ${failures.length === 1 ? 'failure' : 'failures'} · ${suiteCount} ${
            suiteCount === 1 ? 'suite' : 'suites'
          }`}
          {newCount > 0 && ` · ${newCount} new`}
        </span>
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
            openSummaries={openSummaries}
            onToggleSummary={(key) =>
              setOpenSummaries((prev) => ({ ...prev, [key]: !prev[key] }))
            }
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
  openSummaries,
  onToggleSummary,
}: FailureGroupBlockProps) {
  const rows = allRowsShown ? group.rows : group.rows.slice(0, ROWS_PER_GROUP)
  const hiddenCount = group.rows.length - rows.length

  return (
    <div data-testid="run-failure-group">
      <div className="flex items-center gap-2 px-3 py-1.5">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={open}
          className="text-fact hover:text-foreground flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          {open ? (
            <ChevronDown size={14} className="shrink-0" />
          ) : (
            <ChevronRight size={14} className="shrink-0" />
          )}
          <span
            className="text-foreground min-w-0 truncate text-sm font-medium"
            title={group.message}
          >
            {group.message ?? group.title}
          </span>
          {group.subtitle && (
            <span className="text-fact shrink-0 text-xs">{group.subtitle}</span>
          )}
        </button>

        {/* Every contributing build stays reachable, so a sharded suite does
            not hide two thirds of its reports behind one link. */}
        {group.suite?.builds?.map((build) => (
          <NavLink
            key={build.build_id}
            to={`/projects/${group.suite?.project_id ?? 0}/reports/${build.build_number}`}
            className="text-fact hover:text-foreground inline-flex shrink-0 items-center gap-0.5 text-xs hover:underline"
          >
            {`#${build.build_number}`}
            <FileText size={10} />
          </NavLink>
        ))}
      </div>

      {open && (
        <div className="pb-1.5">
          {rows.map((row) => (
            <FailureRow
              key={row.key}
              row={row}
              grouping={grouping}
              llmEnabled={llmEnabled}
              summaryOpen={openSummaries[row.key] ?? false}
              onToggleSummary={() => onToggleSummary(row.key)}
            />
          ))}

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
    </div>
  )
}

interface FailureRowProps {
  row: MergedFailure
  grouping: FailureGrouping
  llmEnabled: boolean
  summaryOpen: boolean
  onToggleSummary: () => void
}

function FailureRow({ row, grouping, llmEnabled, summaryOpen, onToggleSummary }: FailureRowProps) {
  const badges = (
    <div className="flex shrink-0 items-center gap-1">
      <FailureBadges
        flaky={row.flaky}
        retries={row.retries}
        newFailed={row.newFailed}
        known={row.known}
      />
      {llmEnabled && (
        <button
          type="button"
          onClick={onToggleSummary}
          aria-expanded={summaryOpen}
          aria-label={`Toggle AI failure summary for ${row.testName}`}
          className="text-fact hover:text-foreground shrink-0"
        >
          <Sparkles size={12} />
        </button>
      )}
    </div>
  )

  return (
    <div data-testid="run-failure-row">
      <div
        className={cn(
          'hover:bg-muted/40 px-3 py-1 pl-9 text-sm',
          grouping === 'suite' ? GRID_BY_SUITE : GRID_BY_ERROR,
        )}
      >
        <NavLink
          to={`/projects/${row.projectId}/reports/${row.buildNumber}`}
          className="min-w-0 truncate hover:underline"
          title={row.fullName || row.testName}
        >
          {row.testName}
        </NavLink>

        {grouping === 'error' ? (
          <span className="text-fact truncate text-xs" title={row.slug}>
            {row.displayName || row.slug}
          </span>
        ) : null}

        {badges}

        {grouping === 'suite' && row.errorMessage ? <ErrorText message={row.errorMessage} /> : null}
      </div>

      {llmEnabled && summaryOpen && (
        <div className="px-3 pl-9">
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
    <div className="min-w-0 text-xs">
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
