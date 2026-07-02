import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchProjectDefectSummary } from '@/api/defects'
import { queryKeys } from '@/lib/query-keys'
import { Skeleton } from '@/components/ui/skeleton'
import { useProjectDisplay } from '@/features/projects/useProjectDisplay'
import { PageHeader } from '@/components/app/PageHeader'
import { DefectSummaryCards } from './DefectSummaryCards'
import { DefectTrendChart } from './DefectTrendChart'
import { DefectList } from './DefectList'

export function ProjectDefectsView() {
  const { id: projectId } = useParams<{ id: string }>()

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: queryKeys.defectProjectSummary(projectId!),
    queryFn: () => fetchProjectDefectSummary(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  })

  const displayName = useProjectDisplay(projectId)

  if (!projectId) return null

  return (
    <div className="space-y-6">
      <PageHeader title={displayName} subtitle="Defects" />

      {summaryLoading ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      ) : summary ? (
        <DefectSummaryCards summary={summary} />
      ) : null}

      <DefectTrendChart projectId={projectId} />

      <DefectList projectId={projectId} defaultResolution="open" />
    </div>
  )
}
