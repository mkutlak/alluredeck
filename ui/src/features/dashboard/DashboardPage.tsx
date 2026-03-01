import { useQuery } from '@tanstack/react-query'
import { fetchDashboard } from '@/api/dashboard'
import { ProjectStatusCard } from './ProjectStatusCard'
import { Skeleton } from '@/components/ui/skeleton'
import { Card, CardContent } from '@/components/ui/card'

export function DashboardPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard'],
    queryFn: fetchDashboard,
    staleTime: 30_000,
  })

  if (isLoading) {
    return (
      <div className="p-6">
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

  if (!data || data.projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">No projects yet</p>
        <p className="text-sm text-muted-foreground">Create a project to see it here.</p>
      </div>
    )
  }

  const { summary, projects } = data

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <p className="text-sm text-muted-foreground">Overview of all projects</p>
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
        {projects.map((project) => (
          <ProjectStatusCard key={project.project_id} project={project} />
        ))}
      </div>
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
        <span className="mt-1 text-sm text-muted-foreground">{label}</span>
      </CardContent>
    </Card>
  )
}
