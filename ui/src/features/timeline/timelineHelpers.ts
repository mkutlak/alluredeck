// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface TickMark {
  /** Offset in ms from the timeline start (minStart). */
  ms: number
  /** Human-readable label, e.g. "+0s", "+5s", "+1m 30s". */
  label: string
}

// ---------------------------------------------------------------------------
// formatRelativeTime
// ---------------------------------------------------------------------------

/**
 * Formats a millisecond offset from the timeline origin into a human-readable
 * relative label such as "+0s", "+5s", "+1m", or "+1m 30s".
 */
export function formatRelativeTime(ms: number): string {
  const totalSec = Math.floor(ms / 1000)
  if (totalSec < 60) return `+${totalSec}s`
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return s > 0 ? `+${m}m ${s}s` : `+${m}m`
}

// ---------------------------------------------------------------------------
// computeTicks
// ---------------------------------------------------------------------------

/** Nice step intervals in ms, ordered smallest → largest. */
const NICE_INTERVALS_MS = [
  1_000, 2_000, 5_000, 10_000, 15_000, 30_000, 60_000, 120_000, 300_000, 600_000, 1_800_000,
  3_600_000,
]

/**
 * Computes evenly-spaced tick marks for the time axis.
 * Aims for 5–12 ticks using the finest "nice" interval that keeps
 * the total count within that range.
 */
export function computeTicks(minStart: number, maxStop: number): TickMark[] {
  const totalMs = Math.max(maxStop - minStart, 0)

  if (totalMs === 0) {
    return [{ ms: 0, label: formatRelativeTime(0) }]
  }

  // Find the smallest interval that yields ≤ 12 ticks.
  let chosen = NICE_INTERVALS_MS[NICE_INTERVALS_MS.length - 1]
  for (const interval of NICE_INTERVALS_MS) {
    const count = Math.floor(totalMs / interval) + 1
    if (count <= 12) {
      chosen = interval
      break
    }
  }

  const ticks: TickMark[] = []
  for (let ms = 0; ms <= totalMs; ms += chosen) {
    ticks.push({ ms, label: formatRelativeTime(ms) })
  }

  return ticks
}

