import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend } from 'recharts'
import type { StatusTrendPoint } from '@/lib/chart-utils'
import { statusChartConfig } from '@/lib/chart-utils'
import { ChartContainer, ChartTooltipContent, ChartLegendContent } from '@/components/ui/chart'

interface Props {
  data: StatusTrendPoint[]
}

export function StatusTrendChart({ data }: Props) {
  return (
    <ChartContainer config={statusChartConfig} className="h-[240px] w-full">
      <BarChart accessibilityLayer data={data} margin={{ top: 4, right: 8, left: -16, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
        <Tooltip content={<ChartTooltipContent />} />
        <Legend content={<ChartLegendContent />} />
        <Bar dataKey="passed" name="Passed" stackId="a" fill="var(--color-passed)" />
        <Bar dataKey="failed" name="Failed" stackId="a" fill="var(--color-failed)" />
        <Bar dataKey="broken" name="Broken" stackId="a" fill="var(--color-broken)" />
        <Bar dataKey="skipped" name="Skipped" stackId="a" fill="var(--color-skipped)" />
      </BarChart>
    </ChartContainer>
  )
}
