import type { DashboardProjectEntry } from '@/types/api'
import { calcPassRate } from '@/lib/utils'

export type SortField = 'name' | 'type' | 'pass_rate'
export type SortDir = 'asc' | 'desc'
export type ViewMode = 'grouped' | 'all'

export function getProjectType(p: DashboardProjectEntry): string {
  if (p.is_group) return 'Group'
  if (p.report_type === 'playwright') return 'Playwright'
  return 'Allure'
}

export function getPassRate(p: DashboardProjectEntry): number | null {
  // Compute from counts so skipped tests are excluded from the denominator and an
  // all-skipped build returns null (rendered as "—"), consistent with every other surface.
  if (p.is_group) {
    const a = p.aggregate
    return a ? calcPassRate(a.passed, a.total, a.skipped) : null
  }
  const s = p.latest_build?.statistics
  return s ? calcPassRate(s.passed, s.total, s.skipped) : null
}

export function compareRows(
  a: DashboardProjectEntry,
  b: DashboardProjectEntry,
  field: SortField,
  dir: SortDir,
): number {
  const cmp =
    field === 'name'
      ? a.slug.localeCompare(b.slug)
      : field === 'type'
        ? getProjectType(a).localeCompare(getProjectType(b))
        : (getPassRate(a) ?? -1) - (getPassRate(b) ?? -1)
  return dir === 'asc' ? cmp : -cmp
}
