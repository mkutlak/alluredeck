import { Badge, type BadgeProps } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

export interface FlakyBadgeProps extends Omit<BadgeProps, 'variant'> {
  /** Retry count for this occurrence. When > 0, appended to the badge as "· Nx". */
  retries?: number
}

/**
 * Single shared "flaky" badge used across the runs feed, defects, test
 * history, and the flaky-tests card, so flaky tests read consistently
 * wherever they appear.
 */
export function FlakyBadge({ retries, className, ...props }: FlakyBadgeProps) {
  return (
    <Badge
      variant="flaky"
      className={cn('text-xs', className)}
      data-testid="flaky-badge"
      {...props}
    >
      flaky{retries != null && retries > 0 ? ` · ${retries}x` : ''}
    </Badge>
  )
}
