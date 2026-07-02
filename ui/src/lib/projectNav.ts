import type { ProjectEntry } from '@/types/api'

export interface NavItem {
  to: string
  label: string
  end: boolean
  'data-testid': string
}

const ALL_NAV_ITEMS: (Omit<NavItem, 'to'> & { path: string })[] = [
  { label: 'Overview', path: '', end: true, 'data-testid': 'sidebar-nav-overview' },
  { label: 'Analytics', path: '/analytics', end: false, 'data-testid': 'sidebar-nav-analytics' },
  { label: 'Defects', path: '/defects', end: false, 'data-testid': 'sidebar-nav-defects' },
  { label: 'Timeline', path: '/timeline', end: false, 'data-testid': 'sidebar-nav-timeline' },
  {
    label: 'Known Issues',
    path: '/known-issues',
    end: false,
    'data-testid': 'sidebar-nav-known-issues',
  },
  {
    label: 'Attachments',
    path: '/attachments',
    end: false,
    'data-testid': 'sidebar-nav-attachments',
  },
]

// Parents are pure roll-ups: their only view is Pipeline Runs (IA redesign spec,
// 2026-07-02). Child-scoped tabs — including Analytics and Defects, which the old
// sidebar still offered on parents — are intentionally hidden here.
const PARENT_HIDDEN_TABS = ['Analytics', 'Defects', 'Timeline', 'Known Issues', 'Attachments']

/**
 * Builds the project sub-navigation for a given project.
 * Parent projects (with children) get only the index entry, relabeled
 * "Pipeline Runs". Leaf/child projects get the full six-item tab set.
 */
export function projectNavItems(project: ProjectEntry | undefined): NavItem[] {
  if (!project) return []

  const isParent = (project.children?.length ?? 0) > 0
  const items = isParent
    ? ALL_NAV_ITEMS.filter((item) => !PARENT_HIDDEN_TABS.includes(item.label))
    : ALL_NAV_ITEMS

  return items.map(({ label, path, end, 'data-testid': testId }) => ({
    to: `/projects/${project.project_id}${path}`,
    label: isParent && label === 'Overview' ? 'Pipeline Runs' : label,
    end,
    'data-testid': testId,
  }))
}
