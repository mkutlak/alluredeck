import { PieChart, Pie, Tooltip } from 'recharts'
import type { StatusPiePoint } from '@/lib/chart-utils'
import { statusChartConfig } from '@/lib/chart-utils'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'

interface Props {
  data: StatusPiePoint[]
  total: number
}

export function StatusPieChart({ data, total }: Props) {
  // In recharts v3, Cell is deprecated; pass fill directly in each data entry
  const coloredData = data.map((entry) => ({
    ...entry,
    fill: `var(--color-${entry.name.toLowerCase()})`,
  }))

  return (
    <div className="relative">
      <ChartContainer config={statusChartConfig} className="h-[240px] w-full">
        <PieChart accessibilityLayer>
          <Pie
            data={coloredData}
            cx="50%"
            cy="50%"
            innerRadius={60}
            outerRadius={90}
            paddingAngle={2}
            dataKey="value"
          />
          <Tooltip content={<ChartTooltipContent hideLabel />} />
        </PieChart>
      </ChartContainer>
      {/* Center label */}
      <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-bold tabular-nums">{total}</span>
        <span className="text-muted-foreground text-xs">total</span>
      </div>
    </div>
  )
}
