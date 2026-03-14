import { useState } from 'react'
import { Plus, RefreshCw } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { fetchDashboard } from '@/api/dashboard'
import { getTags } from '@/api/projects'
import { queryKeys } from '@/lib/query-keys'
import { ProjectStatusCard } from './ProjectStatusCard'
import { Skeleton } from '@/components/ui/skeleton'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { CreateProjectDialog } from '@/features/projects/CreateProjectDialog'

export function DashboardPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const [selectedTag, setSelectedTag] = useState('')
  const isAdmin = useAuthStore(selectIsAdmin)

  const { data: tagsResp } = useQuery({
    queryKey: queryKeys.tags,
    queryFn: getTags,
    staleTime: 60_000,
  })
  const availableTags = tagsResp?.data ?? []

  const tag = selectedTag || undefined
  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: queryKeys.dashboard(tag),
    queryFn: () => fetchDashboard(tag),
    staleTime: 30_000,
  })

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

      {/* Tag filter bar */}
      {availableTags.length > 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground text-sm">Filter:</span>
          <Badge
            variant={selectedTag === '' ? 'default' : 'outline'}
            className="cursor-pointer"
            onClick={() => setSelectedTag('')}
          >
            All
          </Badge>
          {availableTags.map((t) => (
            <Badge
              key={t}
              variant={selectedTag === t ? 'default' : 'outline'}
              className="cursor-pointer"
              onClick={() => setSelectedTag(t)}
            >
              {t}
            </Badge>
          ))}
        </div>
      )}

      {/* Project grid */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {projects.map((project) => (
          <ProjectStatusCard key={project.project_id} project={project} />
        ))}
      </div>

      <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
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
