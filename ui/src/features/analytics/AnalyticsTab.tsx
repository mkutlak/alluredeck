import { useMemo } from 'react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchReportHistory, fetchReportCategories } from '@/api/reports'
import { fetchTrends } from '@/api/analytics'
import { fetchBranches } from '@/api/branches'
import { queryKeys } from '@/lib/query-keys'
import { toStatusPieData, toCategoryBreakdownData } from '@/lib/chart-utils'
import type { KpiData } from '@/lib/chart-utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { StatusTrendChart } from './StatusTrendChart'
import { PassRateTrendChart } from './PassRateTrendChart'
import { DurationTrendChart } from './DurationTrendChart'
import { StatusPieChart } from './StatusPieChart'
import { CategoryBreakdownChart } from './CategoryBreakdownChart'
import { LowPerformingCard } from './LowPerformingCard'
import { ErrorClusterCard } from './ErrorClusterCard'
import { SuitePassRateChart } from './SuitePassRateChart'
import { LabelBreakdownCard } from './LabelBreakdownCard'
import { AnalyticsSection } from './AnalyticsSection'
import { KpiSummaryRow } from './KpiSummaryRow'
import { useProjectDisplay } from '@/features/projects/useProjectDisplay'
import { useUIStore } from '@/store/ui'
import type { Branch } from '@/types/api'

export function AnalyticsTab() {
  const { id: projectId } = useParams<{ id: string }>()
  const displayName = useProjectDisplay(projectId)
  const branch = useUIStore((s) => s.selectedBranch)
  const setBranch = useUIStore((s) => s.setSelectedBranch)

  // Fetch branches for the selector
  const { data: branchesData } = useQuery({
    queryKey: queryKeys.branches.list(projectId ?? ''),
    queryFn: () => fetchBranches(projectId ?? ''),
    enabled: !!projectId,
  })
  const effectiveBranch =
    branch && branchesData?.some((b) => b.name === branch) ? branch : undefined

  // Fetch pre-computed trend data from the backend.
  const {
    data: trendsData,
    isLoading,
    isError,
  } = useQuery({
    queryKey: queryKeys.trends(projectId ?? '', 100, effectiveBranch),
    queryFn: () => fetchTrends(projectId ?? '', 100, effectiveBranch),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  // Fetch only the latest report for pie chart (1 entry instead of 100).
  const { data: latestData } = useQuery({
    queryKey: queryKeys.reportHistory(projectId ?? '', 1, effectiveBranch, 1),
    queryFn: () => fetchReportHistory(projectId ?? '', 1, 1, effectiveBranch),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  const { data: categoriesData, isLoading: categoriesLoading } = useQuery({
    queryKey: queryKeys.reportCategoriesLatest(projectId ?? ''),
    queryFn: () => fetchReportCategories(projectId ?? ''),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  // Map server snake_case → client camelCase for chart component props.
  const statusTrend = useMemo(() => trendsData?.status ?? [], [trendsData])
  const passRateTrend = useMemo(
    () => (trendsData?.pass_rate ?? []).map((p) => ({ name: p.name, passRate: p.pass_rate })),
    [trendsData],
  )
  const durationTrend = useMemo(
    () => (trendsData?.duration ?? []).map((p) => ({ name: p.name, durationSec: p.duration_sec })),
    [trendsData],
  )

  const latestReports = useMemo(() => latestData?.data.reports ?? [], [latestData])
  const pieData = useMemo(() => toStatusPieData(latestReports), [latestReports])
  const total = latestReports[0]?.statistic?.total ?? 0
  const categoryData = useMemo(
    () => toCategoryBreakdownData(categoriesData ?? []),
    [categoriesData],
  )
  const kpiData = useMemo<KpiData | null>(() => {
    const kpi = trendsData?.kpi
    if (!kpi) return null
    return {
      passRate: kpi.pass_rate,
      passRateTrend: kpi.pass_rate_trend,
      totalTests: kpi.total_tests,
      totalTestsTrend: kpi.total_tests_trend,
      avgDuration: kpi.avg_duration,
      durationTrend: kpi.duration_trend,
      failedCount: kpi.failed_count,
      failedTrend: kpi.failed_trend,
    }
  }, [trendsData])

  const hasTrends = statusTrend.length > 0 || passRateTrend.length > 0 || durationTrend.length > 0
  const hasDistribution = pieData.length > 0 || categoryData.length > 0

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full rounded-lg" />
          ))}
        </div>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-72 w-full rounded-lg" />
          ))}
        </div>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="border-destructive/50 rounded-lg border p-4 text-center">
        <p className="text-destructive text-sm">Failed to load analytics data. Please try again.</p>
      </div>
    )
  }

  if (statusTrend.length === 0 && pieData.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="font-mono text-2xl font-semibold">{displayName}</h1>
            <p className="text-muted-foreground text-sm">Analytics</p>
          </div>
          {branchesData && branchesData.length > 0 && (
            <BranchSelector branches={branchesData} value={branch} onChange={setBranch} />
          )}
        </div>
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <p className="font-medium">No report data yet</p>
          <p className="text-muted-foreground text-sm">
            Generate a report to see analytics charts here.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header with branch selector */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-mono text-2xl font-semibold">{displayName}</h1>
          <p className="text-muted-foreground text-sm">Analytics</p>
        </div>
        {branchesData && branchesData.length > 0 && (
          <BranchSelector branches={branchesData} value={branch} onChange={setBranch} />
        )}
      </div>

      {/* KPI Summary Row */}
      {kpiData && <KpiSummaryRow data={kpiData} />}

      {/* Trends Section */}
      <AnalyticsSection title="Trends" isEmpty={!hasTrends}>
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
        </div>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Duration Trend</CardTitle>
          </CardHeader>
          <CardContent>
            <DurationTrendChart data={durationTrend} />
          </CardContent>
        </Card>
      </AnalyticsSection>

      {/* Quality Section */}
      <AnalyticsSection title="Quality">
        <LowPerformingCard projectId={projectId} branch={branch} />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ErrorClusterCard projectId={projectId} branch={branch} />
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
      </AnalyticsSection>

      {/* Distribution Section */}
      <AnalyticsSection title="Distribution" isEmpty={!hasDistribution}>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Status Distribution</CardTitle>
            </CardHeader>
            <CardContent>
              <StatusPieChart data={pieData} total={total} />
            </CardContent>
          </Card>
          <SuitePassRateChart projectId={projectId} branch={branch} />
          <LabelBreakdownCard projectId={projectId} branch={branch} />
        </div>
      </AnalyticsSection>
    </div>
  )
}

// Branch selector helper component
function BranchSelector({
  branches,
  value,
  onChange,
}: {
  branches: Branch[]
  value: string | undefined
  onChange: (value: string | undefined) => void
}) {
  return (
    <Select
      value={value ?? '__all__'}
      onValueChange={(v) => onChange(v === '__all__' ? undefined : v)}
    >
      <SelectTrigger className="h-8 w-[180px] text-sm">
        <SelectValue placeholder="All branches" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="__all__">All branches</SelectItem>
        {branches.map((b) => (
          <SelectItem key={b.id} value={b.name}>
            {b.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
