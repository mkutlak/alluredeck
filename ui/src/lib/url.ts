/**
 * isSafeUrl checks whether a URL uses a safe protocol (http or https).
 * Rejects javascript:, data:, vbscript:, and other dangerous URI schemes
 * to prevent stored XSS via user-supplied URLs rendered as <a href>.
 */
export function isSafeUrl(url: string): boolean {
  if (!url) return false
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}
