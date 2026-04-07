/**
 * Returns true when the attachment is a Playwright trace archive.
 * Playwright traces are ZIP files whose names start with `trace`.
 */
export function isPlaywrightTrace(name: string, mimeType: string): boolean {
  return (
    (mimeType === 'application/zip' || mimeType === 'application/vnd.allure.playwright-trace') &&
    /^trace/i.test(name)
  )
}
