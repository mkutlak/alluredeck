import { useId } from 'react'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts'
import type { DurationTrendPoint } from '@/lib/chart-utils'
import { durationChartConfig } from '@/lib/chart-utils'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import { formatDuration } from '@/lib/utils'

interface Props {
  data: DurationTrendPoint[]
}

export function DurationTrendChart({ data }: Props) {
  const id = useId()
  const gradientId = `durationGrad${id.replace(/:/g, '')}`
  return (
    <ChartContainer config={durationChartConfig} className="h-[240px] w-full">
      <AreaChart accessibilityLayer data={data} margin={{ top: 4, right: 8, left: -16, bottom: 4 }}>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--color-durationSec)" stopOpacity={0.3} />
            <stop offset="95%" stopColor="var(--color-durationSec)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} tickFormatter={(v: number) => formatDuration(v * 1000)} />
        <Tooltip
          content={<ChartTooltipContent formatter={(v) => formatDuration((v as number) * 1000)} />}
        />
        <Area
          type="monotone"
          dataKey="durationSec"
          name="Duration"
          stroke="var(--color-durationSec)"
          strokeWidth={2}
          fill={`url(#${gradientId})`}
          dot={{ r: 3 }}
        />
      </AreaChart>
    </ChartContainer>
  )
}
