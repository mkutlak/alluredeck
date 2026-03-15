import React from 'react'
import { LineChart, Line } from 'recharts'
import { Card, CardContent } from '@/components/ui/card'
import { formatDuration } from '@/lib/utils'
import type { KpiData } from '@/lib/chart-utils'

export type { KpiData }

interface KpiCardProps {
  label: string
  value: string
  trend: number[]
  color?: string
}

const KpiSparkline = React.memo(function KpiSparkline({
  data,
  color = 'currentColor',
}: {
  data: number[]
  color?: string
}) {
  if (!data || data.length < 2) return null
  const chartData = data.map((v, i) => ({ i, v }))
  return (
    <LineChart data={chartData} width={80} height={32}>
      <Line type="monotone" dataKey="v" dot={false} strokeWidth={1.5} stroke={color} />
    </LineChart>
  )
})

function KpiCard({ label, value, trend, color }: KpiCardProps) {
  return (
    <Card>
      <CardContent className="flex items-center justify-between p-4">
        <div>
          <p className="text-muted-foreground text-xs font-medium">{label}</p>
          <p className="text-2xl font-bold tabular-nums">{value}</p>
        </div>
        <KpiSparkline data={trend} color={color} />
      </CardContent>
    </Card>
  )
}

function getPassRateColorClass(rate: number): string {
  if (rate >= 90) return 'hsl(var(--chart-1))'
  if (rate >= 70) return 'hsl(var(--chart-3))'
  return 'hsl(var(--chart-2))'
}

export function KpiSummaryRow({ data }: { data: KpiData }) {
  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <KpiCard
        label="Pass Rate"
        value={`${data.passRate}%`}
        trend={data.passRateTrend}
        color={getPassRateColorClass(data.passRate)}
      />
      <KpiCard
        label="Total Tests"
        value={String(data.totalTests)}
        trend={data.totalTestsTrend}
        color="hsl(var(--chart-5))"
      />
      <KpiCard
        label="Avg Duration"
        value={formatDuration(data.avgDuration)}
        trend={data.durationTrend}
        color="hsl(var(--chart-5))"
      />
      <KpiCard
        label="Failed"
        value={String(data.failedCount)}
        trend={data.failedTrend}
        color="hsl(var(--chart-2))"
      />
    </div>
  )
}
