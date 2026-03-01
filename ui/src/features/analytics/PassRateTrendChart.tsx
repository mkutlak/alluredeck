import { LineChart, Line, XAxis, YAxis, CartesianGrid, ReferenceLine, Tooltip } from 'recharts'
import type { PassRateTrendPoint } from '@/lib/chart-utils'
import { passRateChartConfig } from '@/lib/chart-utils'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'

interface Props {
  data: PassRateTrendPoint[]
}

export function PassRateTrendChart({ data }: Props) {
  return (
    <ChartContainer config={passRateChartConfig} className="h-[240px] w-full">
      <LineChart accessibilityLayer data={data} margin={{ top: 4, right: 8, left: -16, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis domain={[0, 100]} tick={{ fontSize: 11 }} unit="%" />
        <Tooltip
          content={<ChartTooltipContent formatter={(v) => [`${v}%`, 'Pass Rate']} />}
        />
        <ReferenceLine
          y={90}
          stroke="var(--chart-1)"
          strokeDasharray="4 2"
          label={{ value: '90%', fontSize: 10, fill: 'var(--chart-1)' }}
        />
        <ReferenceLine
          y={70}
          stroke="var(--chart-3)"
          strokeDasharray="4 2"
          label={{ value: '70%', fontSize: 10, fill: 'var(--chart-3)' }}
        />
        <Line
          type="monotone"
          dataKey="passRate"
          name="Pass Rate"
          stroke="var(--color-passRate)"
          strokeWidth={2}
          dot={{ r: 3 }}
          activeDot={{ r: 5 }}
        />
      </LineChart>
    </ChartContainer>
  )
}
