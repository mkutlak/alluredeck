import { Badge } from '@/components/ui/badge'
import { INFO_BADGE_CLASSES, NEUTRAL_BADGE_CLASSES, STATUS_BADGE_CLASSES } from '@/lib/status-colors'

export interface FailureBadgesProps {
  flaky: boolean
  newFailed: boolean
  known: boolean
}

export function FailureBadges({ flaky, newFailed, known }: FailureBadgesProps) {
  return (
    <div className="flex flex-wrap gap-1">
      {flaky && <Badge className={`${STATUS_BADGE_CLASSES.broken} text-[10px]`}>flaky</Badge>}
      {newFailed && <Badge className={`${INFO_BADGE_CLASSES} text-[10px]`}>new</Badge>}
      {known && <Badge className={`${NEUTRAL_BADGE_CLASSES} text-[10px]`}>known</Badge>}
    </div>
  )
}
