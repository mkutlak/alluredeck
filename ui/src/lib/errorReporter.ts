// ---------------------------------------------------------------------------
// Error reporter — pluggable sink for frontend telemetry (Sentry, OTel-web, …)
//
// Usage:
//   import { setErrorReporter, reportError } from '@/lib/errorReporter'
//   setErrorReporter((error, ctx) => Sentry.captureException(error, { extra: ctx.meta }))
// ---------------------------------------------------------------------------

export interface ErrorContext {
  source: string
  meta?: Record<string, unknown>
}

export type ErrorReporter = (error: unknown, context: ErrorContext) => void

const defaultReporter: ErrorReporter = (error, context) => {
  console.error('[alluredeck]', context.source, error, context.meta ?? {})
}

let activeReporter: ErrorReporter = defaultReporter

export function setErrorReporter(fn: ErrorReporter): void {
  activeReporter = fn
}

export function reportError(error: unknown, context: ErrorContext): void {
  try {
    activeReporter(error, context)
  } catch {
    // Never let a broken reporter crash the app — fall back to console.
    console.error('[errorReporter] reporter threw:', error)
  }
}
