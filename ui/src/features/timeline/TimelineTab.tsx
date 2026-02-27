import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchReportTimeline } from '@/api/reports'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { TimelineChart } from './TimelineChart'

export function TimelineTab() {
  const { id: projectId } = useParams<{ id: string }>()

  const { data, isLoading } = useQuery({
    queryKey: ['report-timeline', projectId],
    queryFn: () => fetchReportTimeline(projectId!),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-[400px] w-full rounded-lg" />
      </div>
    )
  }

  const testCases = data?.test_cases ?? []
  const summary = data?.summary

  if (testCases.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
        <p className="font-medium">No timeline data yet</p>
        <p className="text-sm text-muted-foreground">
          Generate a report to see the test execution timeline here.
        </p>
      </div>
    )
  }

  const totalSec = summary ? (summary.total_duration / 1000).toFixed(1) : '—'

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-sm text-muted-foreground">
          Test Timeline · {summary?.total ?? testCases.length} tests · {totalSec}s total
        </p>
      </div>

      {summary?.truncated && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
          Showing first 5,000 of {summary.total.toLocaleString()} test cases (truncated).
        </div>
      )}

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium">Execution Timeline</CardTitle>
        </CardHeader>
        <CardContent>
          <TimelineChart
            testCases={testCases}
            minStart={summary?.min_start ?? testCases[0].start}
            maxStop={summary?.max_stop ?? testCases[testCases.length - 1].stop}
          />
        </CardContent>
      </Card>
    </div>
  )
}
