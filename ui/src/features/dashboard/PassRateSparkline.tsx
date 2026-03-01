import { Line, LineChart, ResponsiveContainer } from 'recharts'
import type { DashboardSparklinePoint } from '@/types/api'

interface Props {
  data: DashboardSparklinePoint[]
}

export function PassRateSparkline({ data }: Props) {
  if (data.length === 0) return null
  return (
    <ResponsiveContainer width="100%" height={48}>
      <LineChart data={data}>
        <Line
          type="monotone"
          dataKey="pass_rate"
          stroke="#2563eb"
          strokeWidth={1.5}
          dot={false}
          isAnimationActive={false}
        />
      </LineChart>
    </ResponsiveContainer>
  )
}
