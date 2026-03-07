import { useMemo } from 'react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchReportHistory, fetchReportCategories } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import {
  toStatusTrendData,
  toPassRateTrendData,
  toDurationTrendData,
  toStatusPieData,
  toCategoryBreakdownData,
} from '@/lib/chart-utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { StatusTrendChart } from './StatusTrendChart'
import { PassRateTrendChart } from './PassRateTrendChart'
import { DurationTrendChart } from './DurationTrendChart'
import { StatusPieChart } from './StatusPieChart'
import { CategoryBreakdownChart } from './CategoryBreakdownChart'
import { LowPerformingCard } from './LowPerformingCard'

export function AnalyticsTab() {
  const { id: projectId } = useParams<{ id: string }>()

  const { data: historyData, isLoading, isError } = useQuery({
    queryKey: queryKeys.reportHistoryAnalytics(projectId!),
    queryFn: () => fetchReportHistory(projectId!, 1, 100),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  const { data: categoriesData, isLoading: categoriesLoading } = useQuery({
    queryKey: queryKeys.reportCategoriesLatest(projectId!),
    queryFn: () => fetchReportCategories(projectId!),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  const reports = useMemo(() => historyData?.data.reports ?? [], [historyData])

  const statusTrend = useMemo(() => toStatusTrendData(reports), [reports])
  const passRateTrend = useMemo(() => toPassRateTrendData(reports), [reports])
  const durationTrend = useMemo(() => toDurationTrendData(reports), [reports])
  const pieData = useMemo(() => toStatusPieData(reports), [reports])
  const total = reports[0]?.statistic?.total ?? 0
  const categoryData = useMemo(() => toCategoryBreakdownData(categoriesData ?? []), [categoriesData])

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-72 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="rounded-lg border border-destructive/50 p-4 text-center">
        <p className="text-sm text-destructive">Failed to load analytics data. Please try again.</p>
      </div>
    )
  }

  if (reports.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
        <p className="font-medium">No report data yet</p>
        <p className="text-sm text-muted-foreground">
          Generate a report to see analytics charts here.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-sm text-muted-foreground">Analytics</p>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Status Trend</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusTrendChart data={statusTrend} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Pass Rate Trend</CardTitle>
          </CardHeader>
          <CardContent>
            <PassRateTrendChart data={passRateTrend} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Duration Trend</CardTitle>
          </CardHeader>
          <CardContent>
            <DurationTrendChart data={durationTrend} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Latest Status Distribution</CardTitle>
          </CardHeader>
          <CardContent>
            <StatusPieChart data={pieData} total={total} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Failure Categories</CardTitle>
          </CardHeader>
          <CardContent>
            {categoriesLoading ? (
              <Skeleton className="h-40 w-full rounded-md" />
            ) : (
              <CategoryBreakdownChart data={categoryData} />
            )}
          </CardContent>
        </Card>
      </div>

      <LowPerformingCard projectId={projectId} />
    </div>
  )
}
