import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PieChart, Pie, Tooltip, Legend } from 'recharts'
import { fetchLabelBreakdown } from '@/api/analytics'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChartContainer, ChartTooltipContent, ChartLegendContent } from '@/components/ui/chart'
import type { ChartConfig } from '@/components/ui/chart'

interface Props {
  projectId: string
  branch?: string
}

const BUILDS = 20
const LABEL_OPTIONS = ['severity', 'feature', 'story', 'owner'] as const
type LabelName = (typeof LABEL_OPTIONS)[number]

/** Generate distinct colors for pie slices using HSL with evenly spaced hues. */
const SLICE_COLORS = [
  'hsl(220 70% 50%)',
  'hsl(160 60% 45%)',
  'hsl(30 80% 55%)',
  'hsl(340 65% 50%)',
  'hsl(270 55% 55%)',
  'hsl(45 85% 50%)',
  'hsl(190 70% 45%)',
  'hsl(0 65% 50%)',
  'hsl(120 50% 45%)',
  'hsl(300 50% 55%)',
]

function buildChartConfig(data: ReadonlyArray<{ value: string }>): ChartConfig {
  const config: ChartConfig = {}
  data.forEach((item, i) => {
    config[item.value] = {
      label: item.value,
      color: SLICE_COLORS[i % SLICE_COLORS.length],
    }
  })
  return config
}

export function LabelBreakdownCard({ projectId, branch }: Props) {
  const [labelName, setLabelName] = useState<LabelName>('severity')

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.labelBreakdown(projectId, labelName, BUILDS, branch),
    queryFn: () =>
      branch !== undefined
        ? fetchLabelBreakdown(projectId, labelName, BUILDS, branch)
        : fetchLabelBreakdown(projectId, labelName, BUILDS),
    staleTime: 60_000,
  })

  const labels = useMemo(() => data?.data ?? [], [data?.data])
  const chartConfig = useMemo(() => buildChartConfig(labels), [labels])

  // In recharts v3, Cell is deprecated; pass fill directly in each data entry
  const coloredData = useMemo(
    () =>
      labels.map((entry, i) => ({
        ...entry,
        name: entry.value,
        fill: SLICE_COLORS[i % SLICE_COLORS.length],
      })),
    [labels],
  )

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-medium">Label Breakdown</CardTitle>
          <Select value={labelName} onValueChange={(v) => setLabelName(v as LabelName)}>
            <SelectTrigger className="h-7 w-[120px] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LABEL_OPTIONS.map((opt) => (
                <SelectItem key={opt} value={opt}>
                  {opt}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-[300px] w-full rounded-md" />
        ) : labels.length === 0 ? (
          <p className="text-muted-foreground py-4 text-center text-sm">No label data available</p>
        ) : (
          <ChartContainer config={chartConfig} className="h-[300px] w-full">
            <PieChart accessibilityLayer>
              <Pie
                data={coloredData}
                cx="50%"
                cy="50%"
                innerRadius={50}
                outerRadius={90}
                paddingAngle={2}
                dataKey="count"
                nameKey="name"
              />
              <Tooltip content={<ChartTooltipContent nameKey="name" hideLabel />} />
              <Legend content={<ChartLegendContent nameKey="name" />} />
            </PieChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}
