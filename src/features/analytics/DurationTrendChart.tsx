import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import type { DurationTrendPoint } from '@/lib/chart-utils'
import { formatDuration } from '@/lib/utils'

interface Props {
  data: DurationTrendPoint[]
}

export function DurationTrendChart({ data }: Props) {
  return (
    <ResponsiveContainer width="100%" height={240}>
      <AreaChart data={data} margin={{ top: 4, right: 8, left: -16, bottom: 4 }}>
        <defs>
          <linearGradient id="durationGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="#2563eb" stopOpacity={0.3} />
            <stop offset="95%" stopColor="#2563eb" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} unit="s" />
        <Tooltip formatter={(v) => [formatDuration((v as number) * 1000), 'Duration']} />
        <Area
          type="monotone"
          dataKey="durationSec"
          name="Duration"
          stroke="#2563eb"
          strokeWidth={2}
          fill="url(#durationGrad)"
          dot={{ r: 3 }}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}
