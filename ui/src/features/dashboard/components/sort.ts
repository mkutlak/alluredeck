import type { DashboardProjectEntry } from '@/types/api'

export type SortField = 'name' | 'type' | 'pass_rate'
export type SortDir = 'asc' | 'desc'
export type ViewMode = 'grouped' | 'all'

/** Returns the human-friendly label for a project: display_name if available, otherwise slug. */
export function projectLabel(p: { slug: string; display_name?: string; project_id: number }) {
  return p.display_name || p.slug || String(p.project_id)
}

export function getProjectType(p: DashboardProjectEntry): string {
  if (p.is_group) return 'Group'
  if (p.report_type === 'playwright') return 'Playwright'
  return 'Allure'
}

export function getPassRate(p: DashboardProjectEntry): number | null {
  if (p.is_group) return p.aggregate?.pass_rate ?? null
  return p.latest_build?.pass_rate ?? null
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
