import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  ReferenceLine,
} from 'recharts'
import type { PassRateTrendPoint } from '@/lib/chart-utils'

interface Props {
  data: PassRateTrendPoint[]
}

export function PassRateTrendChart({ data }: Props) {
  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={data} margin={{ top: 4, right: 8, left: -16, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis domain={[0, 100]} tick={{ fontSize: 11 }} unit="%" />
        <Tooltip formatter={(v) => [`${v}%`, 'Pass Rate']} />
        <ReferenceLine y={90} stroke="#16a34a" strokeDasharray="4 2" label={{ value: '90%', fontSize: 10, fill: '#16a34a' }} />
        <ReferenceLine y={70} stroke="#d97706" strokeDasharray="4 2" label={{ value: '70%', fontSize: 10, fill: '#d97706' }} />
        <Line
          type="monotone"
          dataKey="passRate"
          name="Pass Rate"
          stroke="#2563eb"
          strokeWidth={2}
          dot={{ r: 3 }}
          activeDot={{ r: 5 }}
        />
      </LineChart>
    </ResponsiveContainer>
  )
}
