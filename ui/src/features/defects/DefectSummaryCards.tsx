import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import type { DefectProjectSummary } from '@/types/api'

interface DefectSummaryCardsProps {
  summary: DefectProjectSummary
}

interface StatCard {
  label: string
  value: number
  colorClass: string
}

export function DefectSummaryCards({ summary }: DefectSummaryCardsProps) {
  const cards: StatCard[] = [
    { label: 'Open', value: summary.open, colorClass: STATUS_TEXT_CLASSES.failed },
    { label: 'Fixed', value: summary.fixed, colorClass: STATUS_TEXT_CLASSES.passed },
    { label: 'Muted', value: summary.muted, colorClass: STATUS_TEXT_CLASSES.broken },
    {
      label: 'Regressions',
      value: summary.regressions_last_build,
      colorClass: STATUS_TEXT_CLASSES.failed,
    },
  ]

  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
      {cards.map((card) => (
        <div
          key={card.label}
          className="rounded-lg border p-4"
          data-testid={`summary-${card.label.toLowerCase()}`}
        >
          <p className="text-muted-foreground text-sm">{card.label}</p>
          <p className={`text-2xl font-bold ${card.colorClass}`}>{card.value}</p>
        </div>
      ))}
    </div>
  )
}
