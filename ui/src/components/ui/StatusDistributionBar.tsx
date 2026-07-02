import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export interface StatusDistributionBarProps {
  passed: number
  failed: number
  broken: number
  skipped: number
  className?: string
}

// Solid (non-tinted) status colors — light + dark pairs — for a visible stacked bar.
const SEGMENT_CLASSES = {
  passed: 'bg-[#40a02b] dark:bg-[#a6e3a1]',
  failed: 'bg-[#d20f39] dark:bg-[#f38ba8]',
  broken: 'bg-[#fe640b] dark:bg-[#fab387]',
  skipped: 'bg-[#8c8fa1] dark:bg-[#7f849c]',
} as const

const STATUSES = ['passed', 'failed', 'broken', 'skipped'] as const

export function StatusDistributionBar({
  passed,
  failed,
  broken,
  skipped,
  className,
}: StatusDistributionBarProps) {
  const counts = { passed, failed, broken, skipped }
  const total = passed + failed + broken + skipped
  const label = `${passed} passed, ${failed} failed, ${broken} broken, ${skipped} skipped`

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          role="img"
          aria-label={label}
          className={cn('bg-muted flex h-2 min-w-24 overflow-hidden rounded-full', className)}
        >
          {total > 0 &&
            STATUSES.map((status) => {
              const count = counts[status]
              if (count === 0) return null
              return (
                <span
                  key={status}
                  data-testid="status-segment"
                  data-status={status}
                  style={{ width: `${(count / total) * 100}%` }}
                  className={SEGMENT_CLASSES[status]}
                />
              )
            })}
        </div>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
