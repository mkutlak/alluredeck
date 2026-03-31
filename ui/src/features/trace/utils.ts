/**
 * Returns true when the attachment is a Playwright trace archive.
 * Playwright traces are ZIP files whose names follow the pattern `trace*.zip`.
 */
export function isPlaywrightTrace(name: string, mimeType: string): boolean {
  return mimeType === 'application/zip' && /trace.*\.zip$/i.test(name)
}
