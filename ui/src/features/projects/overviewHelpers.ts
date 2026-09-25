import { calcPassRate } from '@/lib/utils'
import type { Branch, ReportHistoryEntry } from '@/types/api'

/** Synthetic entry the report-history API prepends for the latest report. */
const LATEST_ALIAS_ID = 'latest'

/** Builds that can be picked at once for the compare view. */
const MAX_COMPARE_SELECTION = 2

/**
 * The selected branch is shared across projects, so it only filters this
 * project's history when the project actually has that branch.
 */
export function resolveEffectiveBranch(
  selectedBranch: string | undefined,
  branches: ReadonlyArray<Pick<Branch, 'name'>> | undefined,
): string | undefined {
  return selectedBranch && branches?.some((b) => b.name === selectedBranch)
    ? selectedBranch
    : undefined
}

/** Splits a history page into the "latest" entry and the numbered builds for the table. */
export function splitReportHistory(reports: ReportHistoryEntry[]): {
  latest: ReportHistoryEntry | undefined
  tableReports: ReportHistoryEntry[]
} {
  return {
    latest: reports.find((r) => r.is_latest),
    tableReports: reports.filter((r) => r.report_id !== LATEST_ALIAS_ID),
  }
}

/**
 * Header stat chips must reflect the branch filter, so they come from the
 * newest NUMBERED build of the branch-filtered history, never the synthetic
 * "latest" alias, which the API prepends unfiltered by branch.
 */
export function deriveHeaderChips(chipReports: ReportHistoryEntry[]): {
  chipLatest: ReportHistoryEntry | undefined
  passRate: number | null
} {
  const chipLatest = chipReports.find((r) => r.report_id !== LATEST_ALIAS_ID)
  const stat = chipLatest?.statistic
  const passRate = stat ? calcPassRate(stat.passed, stat.total, stat.skipped) : null
  return { chipLatest, passRate }
}

/** Toggles a build in the compare selection, capped at two builds. */
export function toggleCompareSelection(selected: ReadonlySet<string>, id: string): Set<string> {
  const next = new Set(selected)
  if (next.has(id)) {
    next.delete(id)
  } else if (next.size < MAX_COMPARE_SELECTION) {
    next.add(id)
  }
  return next
}
