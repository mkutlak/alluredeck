import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts'
import type { StatusTrendPoint } from '@/lib/chart-utils'
import { STATUS_COLORS } from '@/lib/chart-utils'

interface Props {
  data: StatusTrendPoint[]
}

export function StatusTrendChart({ data }: Props) {
  return (
    <ResponsiveContainer width="100%" height={240}>
      <BarChart data={data} margin={{ top: 4, right: 8, left: -16, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
        <XAxis dataKey="name" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} allowDecimals={false} />
        <Tooltip />
        <Legend wrapperStyle={{ fontSize: 12 }} />
        <Bar dataKey="passed" name="Passed" stackId="a" fill={STATUS_COLORS.passed} />
        <Bar dataKey="failed" name="Failed" stackId="a" fill={STATUS_COLORS.failed} />
        <Bar dataKey="broken" name="Broken" stackId="a" fill={STATUS_COLORS.broken} />
        <Bar dataKey="skipped" name="Skipped" stackId="a" fill={STATUS_COLORS.skipped} />
      </BarChart>
    </ResponsiveContainer>
  )
}
