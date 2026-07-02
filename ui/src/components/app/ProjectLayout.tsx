import { NavLink, Outlet, useParams } from 'react-router'
import { useProjectFromParam } from '@/lib/resolveProject'
import { formatProjectLabel } from '@/lib/projectLabel'
import { projectNavItems } from '@/lib/projectNav'
import { useTrackActiveTab } from '@/hooks/useTrackActiveTab'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/app/PageHeader'
import { TabsNav } from '@/components/ui/tabs-nav'
import { ProjectActionsMenu } from '@/components/app/ProjectActionsMenu'

export function ProjectLayout() {
  const { id: projectId } = useParams<{ id: string }>()
  const { project, projects, isLoading } = useProjectFromParam(projectId)
  useTrackActiveTab(projectId ?? null)

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

  const navItems = projectNavItems(project)

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
