import type { RunFailure } from '@/types/api'

/** Where a failure is counted: is it me (`new`), was it already broken, or no telling. */
export type FailureBucket = 'new' | 'inherited' | 'unknown'

type Origin = Pick<RunFailure, 'origin' | 'environment'>

/** The API's origin, with a missing or unexpected value ('', a newer one) read as unknown. */
export function normalOrigin(origin: unknown): FailureBucket {
  return origin === 'new' || origin === 'inherited' ? origin : 'unknown'
}

/**
 * Owner decision 2026-10-07, kept in this one place: an environment failure is never
 * "new", it counts as inherited (and is also shown as env). A row without a known
 * origin counts as unknown.
 */
export function failureBucket(row: Origin): FailureBucket {
  return row.environment ? 'inherited' : normalOrigin(row.origin)
}

export interface OriginCounts {
  new: number
  inherited: number
  unknown: number
  /** Environment failures: a subset of `inherited`, never a bucket of their own. */
  env: number
}

export function countOrigins(rows: readonly Origin[]): OriginCounts {
  const counts: OriginCounts = { new: 0, inherited: 0, unknown: 0, env: 0 }
  for (const row of rows) {
    counts[failureBucket(row)]++
    if (row.environment) counts.env++
  }
  return counts
}

/**
 * "3 new · 2 inherited (1 env) · 1 unknown" as parts, zeros left out. When the API
 * truncated the failures (`listed` set), the counts cover only those, said once up front.
 *
 * Owner's rule: when every failure is inherited (none new, none unknown) and all were
 * listed, the parts lead with "none new", so "is it me?" is answered before the counts.
 */
export function originParts(counts: OriginCounts, listed?: number): string[] {
  const noneNew =
    listed === undefined && counts.new === 0 && counts.unknown === 0 && counts.inherited > 0
  const parts = [
    noneNew && 'none new',
    counts.new > 0 && `${counts.new} new`,
    counts.inherited > 0 &&
      `${counts.inherited} inherited${counts.env > 0 ? ` (${counts.env} env)` : ''}`,
    counts.unknown > 0 && `${counts.unknown} unknown`,
  ].filter((part) => part !== false)
  if (listed !== undefined && parts[0] !== undefined) parts[0] = `first ${listed}: ${parts[0]}`
  return parts
}
