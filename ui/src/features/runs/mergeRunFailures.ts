import { failureBucket, normalOrigin } from './failureOrigin'
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
  /** The effective bucket is `new` (failureBucket); the API's `new_failed` is not read. */
  newFailed: boolean
  known: boolean
  errorMessage: string
  origin?: RunFailure['origin']
  environment?: boolean
  /** The API's key for the failure's shape (message, phase, deepest step); travels with `errorMessage`. */
  clusterKey?: string
}

// Copies of one test can disagree because only one history_id may match the baseline.
// As in the API's SQL, a failure in the baseline under either id means it was already
// failing (inherited beats new), and any known origin beats unknown.
const ORIGIN_RANK = { unknown: 0, new: 1, inherited: 2 } as const

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
        newFailed: false,
        known: row.known,
        errorMessage: row.error_message,
        origin: row.origin,
        environment: row.environment,
        clusterKey: row.cluster_key,
      })
      continue
    }

    // The copy carrying the message is the more useful one to link to, so it
    // supplies the build as well, and the cluster key that describes the message.
    if (!existing.errorMessage && row.error_message) {
      existing.errorMessage = row.error_message
      existing.clusterKey = row.cluster_key
      existing.buildId = row.build_id
      existing.buildNumber = row.build_number
      existing.historyId = row.history_id
    }

    existing.retries = Math.max(existing.retries, row.retries)
    existing.flaky = existing.flaky || row.flaky
    existing.known = existing.known || row.known
    existing.environment = existing.environment || row.environment
    if (ORIGIN_RANK[normalOrigin(row.origin)] > ORIGIN_RANK[normalOrigin(existing.origin)]) {
      existing.origin = row.origin
    }
  }

  return order
    .map((key) => byKey.get(key))
    .filter((row): row is MergedFailure => row !== undefined)
    .map((row) => ({ ...row, newFailed: failureBucket(row) === 'new' }))
}
