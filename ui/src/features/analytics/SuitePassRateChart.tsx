import { useQuery } from '@tanstack/react-query'
import { BarChart, Bar, XAxis, YAxis, Tooltip } from 'recharts'
import { fetchSuitePassRates } from '@/api/analytics'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import type { ChartConfig } from '@/components/ui/chart'

interface Props {
  projectId: string
}

const BUILDS = 20

const chartConfig = {
  pass_rate: {
    label: 'Pass Rate (%)',
    color: 'var(--chart-1)',
  },
} satisfies ChartConfig

export function SuitePassRateChart({ projectId }: Props) {
  const { data, isLoading } = useQuery({
    queryKey: queryKeys.suitePassRates(projectId, BUILDS),
    queryFn: () => fetchSuitePassRates(projectId, BUILDS),
    staleTime: 60_000,
  })

  const suites = data?.data ?? []

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">Suite Pass Rates</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-[300px] w-full rounded-md" />
        ) : suites.length === 0 ? (
          <p className="text-muted-foreground py-4 text-center text-sm">No suite data available</p>
        ) : (
          <ChartContainer config={chartConfig} className="h-[300px] w-full">
            <BarChart
              accessibilityLayer
              data={suites}
              margin={{ left: 8, right: 16, top: 4, bottom: 4 }}
            >
              <XAxis
                dataKey="suite"
                tick={{ fontSize: 11 }}
                angle={-30}
                textAnchor="end"
                height={60}
              />
              <YAxis domain={[0, 100]} tick={{ fontSize: 11 }} unit="%" />
              <Tooltip
                content={
                  <ChartTooltipContent
                    formatter={(value, _name, item) => {
                      const payload = item.payload as Record<string, unknown> | undefined
                      const passed = payload?.passed ?? 0
                      const total = payload?.total ?? 0
                      return `${value}% (${String(passed)}/${String(total)} tests passed)`
                    }}
                  />
                }
              />
              <Bar dataKey="pass_rate" fill="var(--color-pass_rate)" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}
