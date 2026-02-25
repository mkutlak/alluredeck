import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, LayoutGrid, List, RefreshCw, FolderX } from 'lucide-react'
import { getProjects } from '@/api/projects'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ProjectCard } from './ProjectCard'
import { CreateProjectDialog } from './CreateProjectDialog'

export function ProjectsPage() {
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const viewMode = useUIStore((s) => s.projectViewMode)
  const setViewMode = useUIStore((s) => s.setProjectViewMode)
  const [createOpen, setCreateOpen] = useState(false)

  const { data, isLoading, isError, refetch, isFetching } = useQuery({
    queryKey: ['projects'],
    queryFn: getProjects,
    staleTime: 30_000,
  })

  const projects = data ? Object.values(data.data) : []

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Projects</h1>
          <p className="text-sm text-muted-foreground">
            {isLoading ? 'Loading…' : `${projects.length} project${projects.length !== 1 ? 's' : ''}`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh"
          >
            <RefreshCw size={15} className={isFetching ? 'animate-spin' : ''} />
          </Button>
          <div className="flex rounded-md border">
            <Button
              variant={viewMode === 'grid' ? 'secondary' : 'ghost'}
              size="icon"
              className="h-8 w-8 rounded-r-none"
              onClick={() => setViewMode('grid')}
              aria-label="Grid view"
            >
              <LayoutGrid size={14} />
            </Button>
            <Button
              variant={viewMode === 'table' ? 'secondary' : 'ghost'}
              size="icon"
              className="h-8 w-8 rounded-l-none"
              onClick={() => setViewMode('table')}
              aria-label="List view"
            >
              <List size={14} />
            </Button>
          </div>
          {isAdmin() && (
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus size={14} />
              New project
            </Button>
          )}
        </div>
      </div>

      {/* Error state */}
      {isError && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load projects. Check the API connection and try again.
        </div>
      )}

      {/* Loading skeleton */}
      {isLoading && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-32 w-full rounded-lg" />
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !isError && projects.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <FolderX size={40} className="text-muted-foreground/50" />
          <div>
            <p className="font-medium">No projects yet</p>
            <p className="text-sm text-muted-foreground">
              {isAdmin()
                ? 'Create a project to start collecting Allure results.'
                : 'Ask an administrator to create a project.'}
            </p>
          </div>
          {isAdmin() && (
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus size={14} />
              Create first project
            </Button>
          )}
        </div>
      )}

      {/* Project grid / list */}
      {!isLoading && !isError && projects.length > 0 && viewMode === 'grid' && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {projects.map((p) => (
            <ProjectCard key={p.project_id} projectId={p.project_id} />
          ))}
        </div>
      )}

      {!isLoading && !isError && projects.length > 0 && viewMode === 'table' && (
        <div className="rounded-lg border">
          {projects.map((p, idx) => (
            <div
              key={p.project_id}
              className={`flex items-center justify-between px-4 py-3 ${idx < projects.length - 1 ? 'border-b' : ''}`}
            >
              <span className="font-mono text-sm">{p.project_id}</span>
              <Button asChild size="sm" variant="ghost">
                <a href={`/projects/${p.project_id}`}>View reports →</a>
              </Button>
            </div>
          ))}
        </div>
      )}

      {isAdmin() && (
        <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
      )}
    </div>
  )
}
