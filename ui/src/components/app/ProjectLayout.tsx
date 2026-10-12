import { NavLink, Outlet, useLocation, useParams } from 'react-router'
import { env } from '@/lib/env'
import { useProjectFromParam } from '@/lib/resolveProject'
import { formatProjectLabel } from '@/lib/projectLabel'
import { projectNavItems } from '@/lib/projectNav'
import { useDocumentTitle } from '@/lib/useDocumentTitle'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/app/PageHeader'
import { TabsNav } from '@/components/ui/tabs-nav'
import { ProjectActionsMenu } from '@/components/app/ProjectActionsMenu'

export function ProjectLayout() {
  const { id: projectId } = useParams<{ id: string }>()
  const { project, projects, isLoading } = useProjectFromParam(projectId)
  const { pathname } = useLocation()

  const navItems = projectNavItems(project)
  // The tab is what follows /projects/:id. Compared by path segment: the URL may name the
  // project by a percent-encoded slug, the tabs by id. Routes ignore case and trailing
  // slashes, so this does too.
  const afterProject = (path: string) =>
    path.replace(/\/+$/, '').split('/').slice(3).join('/').toLowerCase()
  const tab = navItems.find((item) => afterProject(item.to) === afterProject(pathname))
  useDocumentTitle(
    project && tab
      ? `${tab.label} · ${formatProjectLabel(project, projects)} — ${env.appTitle}`
      : undefined,
  )

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-64" data-testid="project-layout-skeleton" />
        <Skeleton className="h-8 w-full" data-testid="project-layout-skeleton" />
      </div>
    )
  }

  if (!project) return null

  const parentProject = project.parent_id
    ? projects?.find((p) => p.project_id === project.parent_id)
    : undefined

  return (
    <div className="space-y-6">
      <PageHeader
        title={formatProjectLabel(project, projects)}
        subtitle={
          parentProject ? (
            <span className="flex items-center gap-1">
              Part of:{' '}
              <NavLink
                to={`/projects/${parentProject.project_id}`}
                className="text-primary hover:underline"
                data-testid="project-parent-link"
              >
                {parentProject.slug}
              </NavLink>
            </span>
          ) : undefined
        }
        actions={<ProjectActionsMenu />}
        toolbar={<TabsNav items={navItems} aria-label="Project sections" />}
      />

      <Outlet />
    </div>
  )
}
