import type { PipelineRun, PipelineSuite } from '@/types/api'

/**
 * A suite fails its run when a test failed, or when its report could not be read:
 * no counts, but status "failed".
 */
export function isFailingSuite(suite: PipelineSuite): boolean {
  return suite.failed > 0 || suite.status === 'failed'
}

export type RunVerdict = 'failed' | 'skipped' | 'passed' | 'awaiting'

const GLYPHS = { failed: '✗', skipped: '○', passed: '✓', awaiting: '○' } as const

/**
 * The run's verdict and headline counts, shared by the feed row and the run page.
 * Every suite skipped means nothing ran: a neutral ○, never the green ✓. That is
 * decided from suite status, not from zero counts, which also describe a build
 * whose stats were unreadable.
 *
 * A run the API says is still uploading (`awaiting_suites`) has no verdict yet: its
 * counts cover only the suites that arrived, so "2/2 passed" may be 2 of 9. It takes the
 * same neutral ○, even with failures, which stay in `failedCount` and `suitesFailing`
 * as facts about what has arrived.
 */
export function summarizeRun(run: PipelineRun) {
  const failedCount = run.suites.reduce((sum, s) => sum + s.failed, 0)
  const suitesFailing = run.suites.filter(isFailingSuite).length
  const allSkipped = run.suites.length > 0 && run.suites.every((s) => s.status === 'skipped')
  const awaiting = Math.max(0, run.aggregate.awaiting_suites ?? 0)
  const verdict: RunVerdict =
    awaiting > 0 ? 'awaiting' : suitesFailing > 0 ? 'failed' : allSkipped ? 'skipped' : 'passed'
  return { verdict, glyph: GLYPHS[verdict], failedCount, suitesFailing, awaiting }
}

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
