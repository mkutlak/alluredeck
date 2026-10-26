import { formatProjectLabel } from '@/lib/projectLabel'
import type { ProjectEntry } from '@/types/api'

// Derive tab label + href from a pathname segment
const TAB_SEGMENTS: Record<string, string> = {
  analytics: 'Analytics',
  'known-issues': 'Known Issues',
  timeline: 'Timeline',
  attachments: 'Attachments',
  defects: 'Defects',
  tests: 'Tests',
  compare: 'Build Comparison',
}

export interface Crumb {
  label: string
  href?: string
  icon?: 'folder' | 'file'
}

/** Sentinel label: the project is still resolving, render placeholders. */
export const LOADING_CRUMB = '__loading__'

export interface BreadcrumbParams {
  id?: string
  reportId?: string
  source?: string
  /** The run page, /runs/:groupId/:runKey. */
  groupId?: string
  runKey?: string
}

// A run key is the pipeline ID, else the commit SHA; the page shows a SHA as its
// first 7 characters. Hex with a letter, 12+ long, is a SHA: pipeline IDs are numbers
// or carry dashes (UUIDs), and a 40-character SHA without a letter is about 1 in 10^8.
function runLabel(runKey: string): string {
  return /^[0-9a-f]{12,}$/i.test(runKey) && /[a-f]/i.test(runKey) ? runKey.slice(0, 7) : runKey
}

export interface ResolvedProject {
  project: ProjectEntry | undefined
  projects: readonly ProjectEntry[] | undefined
  isLoading: boolean
}

/**
 * Builds the breadcrumb trail for a pathname. Returns null on the runs feed
 * ("/") and on pages outside the project hierarchy (/admin, /settings/*), where
 * no breadcrumb bar is shown. Links always use numeric ids.
 */
export function buildBreadcrumbs(
  pathname: string,
  params: BreadcrumbParams,
  { project, projects, isLoading }: ResolvedProject,
): Crumb[] | null {
  if (pathname === '/') return null

  if (params.groupId && params.runKey) {
    if (isLoading) return [{ label: LOADING_CRUMB }]
    return [
      { label: 'Runs', href: '/' },
      {
        label: formatProjectLabel(project, projects) || params.groupId,
        href: `/projects/${project?.project_id ?? params.groupId}`,
        icon: 'folder',
      },
      { label: runLabel(params.runKey) },
    ]
  }

  if (!/^\/projects(\/|$)/i.test(pathname)) return null

  // Always include root
  const crumbs: Crumb[] = [{ label: 'Projects', href: '/projects' }]

  if (!params.id) return crumbs

  // Resolve parent if project has one
  const parentId = project?.parent_id
  if (isLoading) {
    // Signal loading state via special sentinel
    return [{ label: LOADING_CRUMB }]
  }

  if (parentId != null) {
    const parent = projects?.find((p) => p.project_id === parentId)
    crumbs.push({
      label: parent?.display_name ?? parent?.slug ?? String(parentId),
      href: `/projects/${parentId}`,
      icon: 'folder',
    })
  }

  // Current project (non-linked)
  const projectLabel = project?.display_name ?? project?.slug ?? params.id
  crumbs.push({ label: projectLabel, icon: 'file' })

  // Determine sub-route segments after /projects/:id/
  const afterId = pathname.replace(/^\/projects\/[^/]+\/?/i, '')
  const segments = afterId ? afterId.split('/').filter(Boolean) : []

  const firstSeg = segments[0]
  if (!firstSeg) return crumbs

  // Deep sub-routes: reports/:reportId or trace/:source
  if (firstSeg === 'reports' && params.reportId) {
    // Tab segment "Overview" → linked to project overview
    crumbs.push({
      label: 'Overview',
      href: `/projects/${project?.project_id ?? params.id}`,
    })
    crumbs.push({ label: `Report #${params.reportId}` })
    return crumbs
  }

  if (firstSeg === 'trace' && params.source) {
    // Tab segment "Attachments" → linked to attachments tab
    crumbs.push({
      label: 'Attachments',
      href: `/projects/${project?.project_id ?? params.id}/attachments`,
    })
    crumbs.push({ label: decodeURIComponent(params.source) })
    return crumbs
  }

  // Named tab segments
  const tabLabel = TAB_SEGMENTS[firstSeg]
  if (tabLabel) {
    crumbs.push({ label: tabLabel })
    return crumbs
  }

  // Unknown segment — show as plain text
  crumbs.push({ label: firstSeg })
  return crumbs
}
