import { useState, useMemo, useCallback } from 'react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchProjectTimeline } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { BranchSelector } from '@/features/projects/BranchSelector'
import { TimelineChart } from './TimelineChart'
import { DateRangePicker } from './DateRangePicker'
import { BuildCountSelector } from './BuildCountSelector'

export function TimelineTab() {
  const { id: projectId } = useParams<{ id: string }>()

  const [branch, setBranch] = useState<string | undefined>(undefined)
  const [dateFrom, setDateFrom] = useState<string | undefined>(undefined)
  const [dateTo, setDateTo] = useState<string | undefined>(undefined)
  const [buildLimit, setBuildLimit] = useState(1)

  const handleRangeChange = useCallback((from: string | undefined, to: string | undefined) => {
    setDateFrom(from)
    setDateTo(to)
  }, [])

  const hasDateRange = dateFrom !== undefined || dateTo !== undefined

  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.projectTimeline(projectId!, branch, dateFrom, dateTo, buildLimit),
    queryFn: () =>
      fetchProjectTimeline(projectId!, {
        branch,
        from: dateFrom,
        to: dateTo,
        limit: buildLimit,
      }),
    enabled: !!projectId,
    staleTime: 10_000,
  })

  // Flatten all test cases across builds for display
  const allTestCases = useMemo(
    () => data?.builds.flatMap((b) => b.test_cases) ?? [],
    [data],
  )

  // Compute summary stats from the first build (for header display)
  const totalTests = useMemo(
    () => data?.builds.reduce((sum, b) => sum + b.summary.total, 0) ?? 0,
    [data],
  )

  const totalDuration = useMemo(
    () => data?.builds.reduce((sum, b) => sum + b.summary.total_duration, 0) ?? 0,
    [data],
  )

  const anyTruncated = useMemo(
    () => data?.builds.some((b) => b.summary.truncated) ?? false,
    [data],
  )

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-[400px] w-full rounded-lg" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="border-destructive/50 rounded-lg border p-4 text-center">
        <p className="text-destructive text-sm">Failed to load timeline data. Please try again.</p>
      </div>
    )
  }

  const builds = data?.builds ?? []

  if (builds.length === 0 || allTestCases.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-end gap-3">
          <BranchSelector
            projectId={projectId}
            selectedBranch={branch}
            onBranchChange={setBranch}
          />
          <DateRangePicker from={dateFrom} to={dateTo} onRangeChange={handleRangeChange} />
          {hasDateRange && <BuildCountSelector value={buildLimit} onChange={setBuildLimit} />}
        </div>
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <p className="font-medium">No timeline data yet</p>
          <p className="text-muted-foreground text-sm">
            Generate a report to see the test execution timeline here.
          </p>
        </div>
      </div>
    )
  }

  const totalSec = (totalDuration / 1000).toFixed(1)
  const showBuildCountWarning =
    data !== undefined && data.total_builds_in_range > data.builds_returned

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-muted-foreground text-sm">
          Test Timeline · {totalTests} tests · {totalSec}s total
        </p>
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <BranchSelector
          projectId={projectId}
          selectedBranch={branch}
          onBranchChange={setBranch}
        />
        <DateRangePicker from={dateFrom} to={dateTo} onRangeChange={handleRangeChange} />
        {hasDateRange && <BuildCountSelector value={buildLimit} onChange={setBuildLimit} />}
      </div>

      {showBuildCountWarning && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-200">
          Showing {data.builds_returned} of {data.total_builds_in_range} builds in the selected
          range.
        </div>
      )}

      {anyTruncated && (
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
          Some builds have been truncated to the first 5,000 test cases.
        </div>
      )}

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium">Execution Timeline</CardTitle>
        </CardHeader>
        <CardContent>
          <TimelineChart
            testCases={allTestCases}
            minStart={data?.global_min_start ?? allTestCases[0].start}
            maxStop={data?.global_max_stop ?? allTestCases[allTestCases.length - 1].stop}
            builds={builds}
          />
        </CardContent>
      </Card>
    </div>
  )
}
