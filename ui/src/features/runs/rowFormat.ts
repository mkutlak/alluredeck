const TAIL_LENGTH = 8
// Up to this long a branch is never split; it is short enough to read whole.
const WHOLE_MAX = 24

// A long branch keeps its last characters on screen: the head truncates with CSS,
// so the ellipsis lands in the middle and the width of the pane decides how much shows.
export function splitBranch(branch: string): [head: string, tail: string] {
  return branch.length <= WHOLE_MAX
    ? [branch, '']
    : [branch.slice(0, -TAIL_LENGTH), branch.slice(-TAIL_LENGTH)]
}

const SECOND = 1e3
const MINUTE = 60 * SECOND
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const UNITS = [
  ['year', 365 * DAY],
  ['month', 30 * DAY],
  ['day', DAY],
  ['hour', HOUR],
  ['minute', MINUTE],
] as const

const relativeFormat = new Intl.RelativeTimeFormat('en-US', { numeric: 'always' })

// "3 hours ago". Floors, so 59.9 minutes is not "1 hour"; a timestamp from a skewed
// CI clock in the future reads "now". Always numeric: with `numeric: 'auto'` 47 hours
// would read "yesterday", which is a calendar day, not an elapsed time.
export function formatRelativeTime(timestamp: string, now = Date.now()): string {
  const elapsed = Math.max(0, now - Date.parse(timestamp))
  if (elapsed < SECOND) return 'now'
  const [unit, ms] = UNITS.find(([, ms]) => elapsed >= ms) ?? (['second', SECOND] as const)
  return relativeFormat.format(-Math.floor(elapsed / ms), unit)
}
