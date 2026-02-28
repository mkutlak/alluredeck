import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchLowPerformingTests } from '@/api/reports'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { LineChart, Line, ResponsiveContainer, Tooltip } from 'recharts'

interface Props {
  projectId: string
}

type SortMode = 'duration' | 'failure_rate'

function formatMetric(value: number, sort: SortMode): string {
  if (sort === 'duration') {
    if (value >= 60_000) return `${(value / 60_000).toFixed(1)}m`
    if (value >= 1_000) return `${(value / 1_000).toFixed(1)}s`
    return `${Math.round(value)}ms`
  }
  return `${(value * 100).toFixed(1)}%`
}

function MiniSparkline({ data }: { data: number[] }) {
  if (!data || data.length < 2) return <span className="text-xs text-muted-foreground">—</span>
  const chartData = data.map((v, i) => ({ i, v }))
  return (
    <ResponsiveContainer width={60} height={24}>
      <LineChart data={chartData}>
        <Line type="monotone" dataKey="v" dot={false} strokeWidth={1.5} stroke="currentColor" />
        <Tooltip content={() => null} />
      </LineChart>
    </ResponsiveContainer>
  )
}

export function LowPerformingCard({ projectId }: Props) {
  const [sort, setSort] = useState<SortMode>('duration')

  const { data, isLoading } = useQuery({
    queryKey: ['low-performing-tests', projectId, sort],
    queryFn: () => fetchLowPerformingTests(projectId, sort),
    staleTime: 60_000,
  })

  const tests = data?.tests ?? []

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-medium">Low Performing Tests</CardTitle>
          <div className="flex gap-1">
            <Button
              size="sm"
              variant={sort === 'duration' ? 'default' : 'outline'}
              className="h-7 px-2 text-xs"
              onClick={() => setSort('duration')}
            >
              Slowest
            </Button>
            <Button
              size="sm"
              variant={sort === 'failure_rate' ? 'default' : 'outline'}
              className="h-7 px-2 text-xs"
              onClick={() => setSort('failure_rate')}
            >
              Least reliable
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        ) : tests.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No data yet — generate some reports to see trends.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-xs text-muted-foreground">
                  <th className="pb-1 text-left font-medium">Test</th>
                  <th className="pb-1 text-right font-medium">
                    {sort === 'duration' ? 'Avg duration' : 'Failure rate'}
                  </th>
                  <th className="pb-1 text-center font-medium">Builds</th>
                  <th className="pb-1 text-center font-medium">Trend</th>
                </tr>
              </thead>
              <tbody>
                {tests.map((test, i) => (
                  <tr key={test.history_id || i} className="border-b last:border-0">
                    <td className="py-1 pr-2">
                      <span className="line-clamp-1 font-mono text-xs" title={test.full_name}>
                        {test.test_name}
                      </span>
                    </td>
                    <td className="py-1 text-right font-mono text-xs">
                      {formatMetric(test.metric, sort)}
                    </td>
                    <td className="py-1 text-center text-xs text-muted-foreground">
                      {test.build_count}
                    </td>
                    <td className="py-1 text-center">
                      <MiniSparkline data={test.trend} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
