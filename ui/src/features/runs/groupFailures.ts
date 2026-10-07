import { errorSignature } from '@/lib/errorSignature'
import { isFailingSuite } from './rowFormat'
import type { MergedFailure } from './mergeRunFailures'
import type { PipelineSuite } from '@/types/api'

export interface FailureGroup {
  key: string
  title: string
  /** Secondary line: shard/build context, or which suites a cluster spans. */
  subtitle?: string
  /** Error groups only: the first non-empty line of the first real message, verbatim; `title` is just the grouping signature. */
  message?: string
  rows: MergedFailure[]
  /** Present only for suite groups — drives the per-build report links. */
  suite?: PipelineSuite
}

/**
 * Groups failures by the suite that produced them, worst first.
 *
 * This is the default because it matches how you navigate afterwards: you open
 * the suite's report. The suite name moves to a header, which removes a whole
 * column from every row.
 */
export function groupBySuite(
  failures: readonly MergedFailure[],
  suites: readonly PipelineSuite[],
): FailureGroup[] {
  const suiteById = new Map(suites.map((s) => [s.project_id, s]))
  const order: number[] = []
  const byProject = new Map<number, MergedFailure[]>()

  for (const failure of failures) {
    const existing = byProject.get(failure.projectId)
    if (existing) {
      existing.push(failure)
      continue
    }
    order.push(failure.projectId)
    byProject.set(failure.projectId, [failure])
  }

  return order
    .map((projectId) =>
      suiteGroup(projectId, byProject.get(projectId) ?? [], suiteById.get(projectId)),
    )
    .sort((a, b) => b.rows.length - a.rows.length || a.title.localeCompare(b.title))
}

function suiteGroup(
  projectId: number,
  rows: MergedFailure[],
  suite: PipelineSuite | undefined,
): FailureGroup {
  const group: FailureGroup = {
    key: `suite-${projectId}`,
    title: suite?.display_name || suite?.slug || rows[0]?.slug || String(projectId),
    rows,
  }
  if (suite) group.suite = suite
  const shardCount = suite?.builds?.length ?? 1
  if (shardCount > 1) group.subtitle = `${shardCount} shards`
  return group
}

/**
 * The run page's suite sections: every failing suite, including one whose rows the
 * API's failure cap left out, so a chip's `#suite-<id>` always lands. Worst first by
 * the suite's real failed count, as the chips are, not by how many rows arrived.
 */
export function suiteSections(
  failures: readonly MergedFailure[],
  suites: readonly PipelineSuite[],
): FailureGroup[] {
  const groups = groupBySuite(failures, suites)
  const listed = new Set(groups.map((g) => g.key))
  for (const suite of suites) {
    if (isFailingSuite(suite) && !listed.has(`suite-${suite.project_id}`)) {
      groups.push(suiteGroup(suite.project_id, [], suite))
    }
  }
  const failed = (g: FailureGroup) => g.suite?.failed ?? g.rows.length
  return groups.sort((a, b) => failed(b) - failed(a) || a.title.localeCompare(b.title))
}

/** First non-empty line, verbatim: a stack trace must not become a title. */
function firstLine(message: string): string {
  return (
    message
      .split('\n')
      .find((line) => line.trim() !== '')
      ?.trim() ?? ''
  )
}

/**
 * Groups failures by normalised error signature, worst first.
 *
 * A single run routinely repeats one message across a dozen-plus tests; this
 * view states that once and lists what it hit. The signature only decides the
 * grouping: the group is shown under the first real message, verbatim.
 */
export function groupByError(failures: readonly MergedFailure[]): FailureGroup[] {
  const order: string[] = []
  const bySignature = new Map<string, MergedFailure[]>()

  for (const failure of failures) {
    const signature = errorSignature(failure.errorMessage)
    const existing = bySignature.get(signature)
    if (existing) {
      existing.push(failure)
      continue
    }
    order.push(signature)
    bySignature.set(signature, [failure])
  }

  return order
    .map((signature) => {
      const rows = bySignature.get(signature) ?? []
      // Keyed on project, not name: two projects can share a display name.
      const suites = [...new Map(rows.map((r) => [r.projectId, r.displayName || r.slug])).values()]
      const named = suites.slice(0, 2).join(', ')
      const group: FailureGroup = {
        key: `error-${signature}`,
        title: signature,
        subtitle: `×${rows.length} · ${named}${suites.length > 2 ? ` +${suites.length - 2}` : ''}`,
        rows,
      }
      const message = rows.map((r) => firstLine(r.errorMessage)).find(Boolean)
      if (message) group.message = message
      return group
    })
    .sort((a, b) => b.rows.length - a.rows.length || a.title.localeCompare(b.title))
}
