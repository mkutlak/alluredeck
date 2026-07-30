/** Bucket label for failures that carry no error message at all. */
export const NO_ERROR_SIGNATURE = 'No error message'

const MAX_SIGNATURE_LENGTH = 120

/**
 * Reduces a failure message to a stable key that groups the same failure mode
 * across tests.
 *
 * A single run routinely produces the same message dozens of times, differing
 * only in a timeout value or a quoted value, so listing them individually
 * repeats one fact many times. Normalising the volatile parts turns those into
 * one cluster while leaving genuinely different failures apart.
 *
 * The heuristic deliberately lives in the UI: it is presentation-only, and
 * tuning it must not require an API contract change.
 */
export function errorSignature(message: string): string {
  const firstLine = message.split('\n')[0]?.replace(/\r$/, '') ?? ''

  const normalised = firstLine
    // Quoted values differ per test case but describe one assertion.
    .replace(/'[^']*'/g, "'…'")
    .replace(/"[^"]*"/g, '"…"')
    // Timeouts, counts, line numbers: the number is never what distinguishes
    // one failure mode from another.
    .replace(/\d+/g, 'N')
    .replace(/\s+/g, ' ')
    .trim()

  if (normalised === '') return NO_ERROR_SIGNATURE

  return normalised.length > MAX_SIGNATURE_LENGTH
    ? `${normalised.slice(0, MAX_SIGNATURE_LENGTH - 1)}…`
    : normalised
}
