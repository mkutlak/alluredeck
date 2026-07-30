import type { RunFailure } from '@/types/api'

/** One failing test after duplicate ingestion copies have been folded together. */
export interface MergedFailure {
  key: string
  projectId: number
  slug: string
  displayName?: string
  buildId: number
  buildNumber: number
  testName: string
  fullName: string
  historyId: string
  flaky: boolean
  retries: number
  newFailed: boolean
  known: boolean
  errorMessage: string
}

/**
 * Folds duplicate copies of the same failing test into one row.
 *
 * A build can hold two rows for one test because two ingestion paths assign it
 * different history_ids (one joins the hash parts with ".", the other with
 * ":"), and typically only one copy carries the error message. The partial
 * unique index on (build_id, history_id) does not catch this, and the only
 * field the copies share is full_name.
 *
 * Merging happens here rather than in SQL because full_name is not a safe
 * dedupe key in general — parameterized tests legitimately share one — and
 * ListFailedByBuild backs the MCP tools and known-issue matching as well. This
 * is a presentation-level merge, scoped to the runs feed.
 */
export function mergeRunFailures(rows: readonly RunFailure[]): MergedFailure[] {
  const order: string[] = []
  const byKey = new Map<string, MergedFailure>()

  for (const row of rows) {
    // Fall back to test_name for producers that leave full_name empty.
    const identity = row.full_name || row.test_name
    const key = `${row.project_id}|${identity}`

    const existing = byKey.get(key)
    if (!existing) {
      order.push(key)
      byKey.set(key, {
        key,
        projectId: row.project_id,
        slug: row.slug,
        displayName: row.display_name,
        buildId: row.build_id,
        buildNumber: row.build_number,
        testName: row.test_name,
        fullName: row.full_name,
        historyId: row.history_id,
        flaky: row.flaky,
        retries: row.retries,
        newFailed: row.new_failed,
        known: row.known,
        errorMessage: row.error_message,
      })
      continue
    }

    // The copy carrying the message is the more useful one to link to, so it
    // supplies the build as well.
    if (!existing.errorMessage && row.error_message) {
      existing.errorMessage = row.error_message
      existing.buildId = row.build_id
      existing.buildNumber = row.build_number
      existing.historyId = row.history_id
    }

    existing.retries = Math.max(existing.retries, row.retries)
    existing.flaky = existing.flaky || row.flaky
    existing.newFailed = existing.newFailed || row.new_failed
    existing.known = existing.known || row.known
  }

  return order.map((key) => byKey.get(key)).filter((row): row is MergedFailure => row !== undefined)
}
