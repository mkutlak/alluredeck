import { Link, useLocation, useParams } from 'react-router'
import { FileText, Folder } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { useProjectFromParam } from '@/lib/resolveProject'
import { BranchSelector } from '@/features/projects/BranchSelector'
import { useUIStore } from '@/store/ui'

const BRANCH_RELEVANT_SEGMENTS = new Set(['', 'analytics', 'timeline', 'tests'])

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

interface Crumb {
  label: string
  href?: string
  icon?: React.ReactNode
}

function useBreadcrumbs(): Crumb[] | null {
  const location = useLocation()
  const params = useParams<{ id?: string; reportId?: string; source?: string }>()
  const { project, projects, isLoading } = useProjectFromParam(params.id)

  if (location.pathname === '/') return null

  // Always include root
  const crumbs: Crumb[] = [{ label: 'Projects', href: '/' }]

  if (!params.id) return crumbs

  // Resolve parent if project has one
  const parentId = project?.parent_id
  if (isLoading) {
    // Signal loading state via special sentinel
    return [{ label: '__loading__' }]
  }

  if (parentId != null) {
    const parent = projects?.find((p) => p.project_id === parentId)
    crumbs.push({
      label: parent?.display_name ?? parent?.slug ?? String(parentId),
      href: `/projects/${parentId}`,
      icon: <Folder size={14} className="text-muted-foreground" />,
    })
  }

  // Current project (non-linked)
  const projectLabel = project?.display_name ?? project?.slug ?? params.id
  crumbs.push({
    label: projectLabel,
    icon: <FileText size={14} />,
  })

  // Determine sub-route segments after /projects/:id/
  const afterId = location.pathname.replace(/^\/projects\/[^/]+\/?/, '')
  const segments = afterId ? afterId.split('/').filter(Boolean) : []

  if (segments.length === 0) return crumbs

  const firstSeg = segments[0]

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

export function BreadcrumbBar() {
  const location = useLocation()
  const params = useParams<{ id?: string }>()
  const { isLoading } = useProjectFromParam(params.id)
  const crumbs = useBreadcrumbs()
  const selectedBranch = useUIStore((s) => s.selectedBranch)
  const setSelectedBranch = useUIStore((s) => s.setSelectedBranch)

  if (location.pathname === '/') return null
  if (!crumbs) return null

  const isLoadingState = crumbs.length === 1 && crumbs[0].label === '__loading__'

  // Determine if the current route is branch-relevant
  const afterId = params.id
    ? location.pathname.replace(/^\/projects\/[^/]+\/?/, '')
    : null
  const firstSeg = afterId != null ? (afterId.split('/')[0] ?? '') : null
  const showBranchSelector =
    params.id != null &&
    firstSeg != null &&
    BRANCH_RELEVANT_SEGMENTS.has(firstSeg) &&
    !isLoadingState &&
    !isLoading

  return (
    <nav
      aria-label="Breadcrumb"
      className="border-b bg-background flex h-10 shrink-0 items-center gap-1.5 px-4 text-sm"
    >
      {isLoadingState || isLoading ? (
        <>
          <Skeleton className="h-4 w-16" data-testid="breadcrumb-skeleton" />
          <span className="text-muted-foreground">/</span>
          <Skeleton className="h-4 w-24" data-testid="breadcrumb-skeleton" />
        </>
      ) : (
        <>
          <ol className="flex items-center gap-1.5">
            {crumbs.map((crumb, i) => {
              const isLast = i === crumbs.length - 1
              return (
                <li key={i} className="flex items-center gap-1.5">
                  {i > 0 && <span className="text-muted-foreground select-none">/</span>}
                  {crumb.icon && (
                    <span className="flex items-center">{crumb.icon}</span>
                  )}
                  {crumb.href && !isLast ? (
                    <Link
                      to={crumb.href}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {crumb.label}
                    </Link>
                  ) : (
                    <span className={isLast ? 'font-medium' : 'text-muted-foreground'}>
                      {crumb.label}
                    </span>
                  )}
                </li>
              )
            })}
          </ol>
          {showBranchSelector && (
            <div className="ml-auto">
              <BranchSelector
                projectId={params.id!}
                selectedBranch={selectedBranch}
                onBranchChange={setSelectedBranch}
              />
            </div>
          )}
        </>
      )}
    </nav>
  )
}
