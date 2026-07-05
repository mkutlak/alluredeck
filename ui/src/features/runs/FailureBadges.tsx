import { Badge } from '@/components/ui/badge'
import { FlakyBadge } from '@/components/ui/FlakyBadge'
import { INFO_BADGE_CLASSES, NEUTRAL_BADGE_CLASSES } from '@/lib/status-colors'

export interface FailureBadgesProps {
  flaky: boolean
  newFailed: boolean
  known: boolean
  retries?: number
}

export function FailureBadges({ flaky, newFailed, known, retries }: FailureBadgesProps) {
  return (
    <div className="flex flex-wrap gap-1">
      {flaky && <FlakyBadge retries={retries} className="text-[10px]" />}
      {newFailed && <Badge className={`${INFO_BADGE_CLASSES} text-[10px]`}>new</Badge>}
      {known && <Badge className={`${NEUTRAL_BADGE_CLASSES} text-[10px]`}>known</Badge>}
    </div>
  )
}
