import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import type { StatusPiePoint } from '@/lib/chart-utils'

interface Props {
  data: StatusPiePoint[]
  total: number
}

export function StatusPieChart({ data, total }: Props) {
  return (
    <div className="relative">
      <ResponsiveContainer width="100%" height={240}>
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            innerRadius={60}
            outerRadius={90}
            paddingAngle={2}
            dataKey="value"
          >
            {data.map((entry) => (
              <Cell key={entry.name} fill={entry.color} />
            ))}
          </Pie>
          <Tooltip formatter={(v, name) => [v, name]} />
        </PieChart>
      </ResponsiveContainer>
      {/* Center label */}
      <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-bold tabular-nums">{total}</span>
        <span className="text-xs text-muted-foreground">total</span>
      </div>
    </div>
  )
}
