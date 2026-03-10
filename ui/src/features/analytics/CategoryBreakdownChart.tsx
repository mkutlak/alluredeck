import { BarChart, Bar, XAxis, YAxis, Tooltip } from 'recharts'
import { type CategoryBreakdownPoint, categoryChartConfig } from '@/lib/chart-utils'
import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'

interface Props {
  data: CategoryBreakdownPoint[]
}

export function CategoryBreakdownChart({ data }: Props) {
  if (data.length === 0) {
    return <p className="text-muted-foreground py-4 text-center text-sm">No defect categories</p>
  }

  const height = Math.max(80, data.length * 52)

  return (
    <div className="space-y-3">
      <ChartContainer
        config={categoryChartConfig}
        className="w-full"
        style={{ height: `${height}px` }}
      >
        <BarChart
          accessibilityLayer
          data={data}
          layout="vertical"
          margin={{ left: 8, right: 16, top: 4, bottom: 4 }}
        >
          <YAxis type="category" dataKey="name" width={120} tick={{ fontSize: 12 }} />
          <XAxis type="number" hide />
          <Tooltip content={<ChartTooltipContent />} />
          <Bar dataKey="failed" stackId="a" fill="var(--color-failed)" name="Failed" />
          <Bar
            dataKey="broken"
            stackId="a"
            fill="var(--color-broken)"
            name="Broken"
            radius={[2, 2, 2, 2]}
          />
        </BarChart>
      </ChartContainer>

      {/* Accessible summary — also serves as test anchor for category names */}
      <ul className="space-y-0.5">
        {data.map((d) => (
          <li key={d.name} className="flex items-center justify-between text-xs">
            <span className="flex items-center gap-1.5">
              <span
                className="inline-block h-2 w-2 shrink-0 rounded-full"
                style={{ backgroundColor: d.color }}
              />
              <span>{d.name}</span>
            </span>
            <span className="text-muted-foreground font-mono">{d.total}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
