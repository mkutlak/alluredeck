import { useState } from 'react'
import { NavLink } from 'react-router'
import { ChevronDown, ChevronRight, MoreHorizontal, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { dashboardOptions } from '@/lib/queries'
import { ProjectStatusCard } from './ProjectStatusCard'
import { Skeleton } from '@/components/ui/skeleton'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { CreateProjectDialog } from '@/features/projects/CreateProjectDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import type { DashboardProjectEntry } from '@/types/api'

export function DashboardPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const isAdmin = useAuthStore(selectIsAdmin)

  const { data, isLoading, isFetching, isError, refetch } = useQuery(dashboardOptions())

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="mb-6">
          <h1 className="text-2xl font-bold">Projects Dashboard</h1>
        </div>
        <div className="mb-6 grid grid-cols-4 gap-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-20 animate-pulse rounded-lg" />
          ))}
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[...Array(6)].map((_, i) => (
            <Skeleton key={i} className="h-56 animate-pulse rounded-lg" />
          ))}
        </div>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">Failed to load dashboard.</p>
        <Button onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  if (!data || data.projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">No projects yet</p>
        <p className="text-muted-foreground text-sm">
          {isAdmin ? 'Create a project to get started.' : 'Ask an admin to create a project.'}
        </p>
        {isAdmin && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus />
            Create first project
          </Button>
        )}
        <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
      </div>
    )
  }

  const { summary, projects } = data

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Projects Dashboard</h1>
          <p className="text-muted-foreground text-sm">Overview of all projects</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="icon" onClick={() => refetch()} aria-label="Refresh">
            <RefreshCw className={isFetching ? 'animate-spin' : ''} />
          </Button>
          {isAdmin && (
            <Button onClick={() => setCreateOpen(true)}>
              <Plus />
              New project
            </Button>
          )}
        </div>
      </div>

      {/* Summary cards */}
      <div className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
        <SummaryCard label="Total Projects" value={summary.total_projects} />
        <SummaryCard label="Healthy" value={summary.healthy} className="text-green-600" />
        <SummaryCard label="Degraded" value={summary.degraded} className="text-amber-500" />
        <SummaryCard label="Failing" value={summary.failing} className="text-destructive" />
      </div>

      {/* Project grid */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {projects.map((project) =>
          project.is_group ? (
            <ProjectGroup key={project.project_id} project={project} />
          ) : (
            <ProjectStatusCard key={project.project_id} project={project} />
          ),
        )}
      </div>

      <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}

function ProjectGroup({ project }: { project: DashboardProjectEntry }) {
  const [expanded, setExpanded] = useState(false)
  const [cleanMode, setCleanMode] = useState<'results' | 'history' | null>(null)
  const isAdmin = useAuthStore(selectIsAdmin)
  const { aggregate } = project
  const children = project.children ?? []

  return (
    <div className="sm:col-span-2 lg:col-span-3">
      <Card>
        <CardContent className="p-4">
          <div className="flex items-center justify-between gap-2">
            <button
              className="flex flex-1 items-center gap-2 text-left"
              onClick={() => setExpanded((v) => !v)}
              aria-expanded={expanded}
            >
              <div className="flex items-center gap-2">
                {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                <NavLink
                  to={`/projects/${project.project_id}`}
                  className="font-semibold hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {project.project_id}
                </NavLink>
                {aggregate && (
                  <Badge variant="secondary" className="text-xs font-normal">
                    {children.length} {children.length === 1 ? 'suite' : 'suites'} ·{' '}
                    {aggregate.pass_rate.toFixed(0)}% pass rate
                  </Badge>
                )}
              </div>
            </button>

            <div className="flex items-center gap-2">
              {aggregate && (
                <span className="text-muted-foreground text-sm">{aggregate.total} total tests</span>
              )}
              {isAdmin && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Group actions"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <MoreHorizontal size={16} />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setCleanMode('results')}
                    >
                      <Trash2 size={14} className="mr-2" />
                      Clean all results
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setCleanMode('history')}
                    >
                      <Trash2 size={14} className="mr-2" />
                      Clean all history
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>
          </div>

          {expanded && children.length > 0 && (
            <div className="mt-4 grid grid-cols-1 gap-4 pl-6 sm:grid-cols-2 lg:grid-cols-3">
              {children.map((child) => (
                <ProjectStatusCard key={child.project_id} project={child} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {cleanMode && (
        <CleanDialog
          projectId={project.project_id}
          mode={cleanMode}
          open={!!cleanMode}
          onOpenChange={(open) => {
            if (!open) setCleanMode(null)
          }}
          groupMode
        />
      )}
    </div>
  )
}

function SummaryCard({
  label,
  value,
  className,
}: {
  label: string
  value: number
  className?: string
}) {
  return (
    <Card>
      <CardContent className="flex flex-col items-center justify-center p-4 text-center">
        <span className={`text-3xl font-bold ${className ?? ''}`}>{value}</span>
        <span className="text-muted-foreground mt-1 text-sm">{label}</span>
      </CardContent>
    </Card>
  )
}
