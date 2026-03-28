import { useParams, Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { fetchBuildDefectSummary } from '@/api/defects'
import { queryKeys } from '@/lib/query-keys'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { DefectList } from './DefectList'

export function BuildDefectsView() {
  const { id: projectId, buildId: buildIdParam } = useParams<{ id: string; buildId: string }>()
  const buildId = Number(buildIdParam)

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: queryKeys.defectBuildSummary(projectId!, buildId),
    queryFn: () => fetchBuildDefectSummary(projectId!, buildId),
    enabled: !!projectId && !Number.isNaN(buildId),
    staleTime: 30_000,
  })

  if (!projectId || Number.isNaN(buildId)) {
    return (
      <div className="p-4 text-center">
        <p className="text-destructive text-sm">Invalid project or build ID.</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div>
        <Link
          to={`/projects/${encodeURIComponent(projectId)}/defects`}
          className="text-muted-foreground hover:text-foreground mb-2 inline-flex items-center gap-1 text-sm"
        >
          <ArrowLeft size={14} />
          Back to project defects
        </Link>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-muted-foreground text-sm">Build #{buildId} — Defects</p>
      </div>

      {summaryLoading ? (
        <div className="flex gap-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-8 w-24" />
          ))}
        </div>
      ) : summary ? (
        <div className="flex flex-wrap gap-3">
          <Badge variant="outline">Groups: {summary.total_groups}</Badge>
          <Badge variant="outline">Affected tests: {summary.affected_tests}</Badge>
          <Badge variant="destructive">New: {summary.new_defects}</Badge>
          <Badge variant="destructive">Regressions: {summary.regressions}</Badge>
        </div>
      ) : null}

      <DefectList projectId={projectId} buildId={buildId} />
    </div>
  )
}
