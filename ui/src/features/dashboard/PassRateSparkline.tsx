import { Line, LineChart } from 'recharts'
import type { DashboardSparklinePoint } from '@/types/api'
import { sparklineChartConfig } from '@/lib/chart-utils'
import { ChartContainer } from '@/components/ui/chart'

interface Props {
  data: DashboardSparklinePoint[]
}

export function PassRateSparkline({ data }: Props) {
  if (data.length === 0) return null
  return (
    <ChartContainer config={sparklineChartConfig} className="h-12 w-full">
      <LineChart data={data}>
        <Line
          type="monotone"
          dataKey="pass_rate"
          stroke="var(--color-passRate)"
          strokeWidth={1.5}
          dot={false}
          isAnimationActive={false}
        />
      </LineChart>
    </ChartContainer>
  )
}
