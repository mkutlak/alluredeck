import { Badge } from '@/components/ui/badge'
import { FlakyBadge } from '@/components/ui/FlakyBadge'
import { INFO_BADGE_CLASSES, NEUTRAL_BADGE_CLASSES } from '@/lib/status-colors'

export interface FailureBadgesProps {
  flaky: boolean
  /** The effective bucket is `new` (see failureOrigin). */
  newFailed: boolean
  known: boolean
  retries?: number
  /** Failing on the default branch too. */
  inherited?: boolean
  /** An infrastructure failure: shown as env, never as new or inherited. */
  environment?: boolean
}

// Labels, not controls: the outline variant has no hover fill (the default variant
// fills solid blue on hover). "inherited" and "env" both say "not from this change",
// so both keep a visible outline and no tint, which keeps them apart from the tinted
// new (blue), flaky (peach) and known (gray) badges. Those three keep their hue in the
// tint (and in dark mode) but use ink for light-mode text: 10px hue text on a 15% tint
// was 2.3-3.8:1. inherited is the quietest, in secondary ink; env, which names a cause,
// is in full ink.
const OUTLINE = 'border-border bg-transparent text-[10px]'

export function FailureBadges({
  flaky,
  newFailed,
  known,
  retries,
  inherited = false,
  environment = false,
}: FailureBadgesProps) {
  return (
    <div className="flex flex-wrap gap-1">
      {flaky && (
        <FlakyBadge
          retries={retries}
          className="text-foreground text-[10px]"
          title="Flaky: this test passed on a retry or fails intermittently"
        />
      )}
      {environment ? (
        <Badge
          variant="outline"
          className={`${OUTLINE} text-foreground`}
          title="Environment (infrastructure) failure: counted as inherited, never as new"
        >
          env
          {/* The tooltip alone reaches neither touch nor screen readers. */}
          <span className="sr-only"> (environment failure)</span>
        </Badge>
      ) : newFailed ? (
        <Badge
          variant="outline"
          className={`${INFO_BADGE_CLASSES} text-foreground text-[10px]`}
          title="New: this test passed on the default branch"
        >
          new
          <span className="sr-only"> (passed on the default branch)</span>
        </Badge>
      ) : (
        inherited && (
          <Badge
            variant="outline"
            className={`${OUTLINE} text-fact`}
            title="Inherited: this test is failing on the default branch too"
          >
            inherited
            <span className="sr-only"> (also failing on the default branch)</span>
          </Badge>
        )
      )}
      {known && (
        <Badge
          variant="outline"
          className={`${NEUTRAL_BADGE_CLASSES} text-foreground text-[10px]`}
          title="Known issue: this test matches a known issue declared for its project"
        >
          known
          <span className="sr-only"> (matches a known issue)</span>
        </Badge>
      )}
    </div>
  )
}
