import { useQueries } from '@tanstack/react-query'
import { NavLink } from 'react-router'

import { buildFailedTestsOptions } from '@/lib/queries'
import { FailureBadges } from './FailureBadges'
import type { PipelineRun } from '@/types/api'

export interface RunFailuresProps {
  run: PipelineRun
}

interface FailureRow {
  key: string
  testName: string
  suiteSlug: string
  suiteHref: string
  flaky: boolean
  newFailed: boolean
  known: boolean
  errorMessage: string
  errorMessageFirstLine: string
}

export function RunFailures({ run }: RunFailuresProps) {
  const failingSuites = run.suites.filter((s) => s.failed > 0)

  const queries = useQueries({
    queries: failingSuites.map((s) => buildFailedTestsOptions(s.project_id, s.build_id)),
  })

  if (failingSuites.length === 0) {
    return null
  }

  const isLoading = queries.some((q) => q.isLoading)
  const isError = queries.some((q) => q.isError)

  if (isLoading) {
    return (
      <div className="text-muted-foreground mt-3 text-sm" data-testid="run-failures">
        Loading failures…
      </div>
    )
  }

  if (isError) {
    return (
      <div className="text-destructive mt-3 text-sm" data-testid="run-failures">
        Failed to load failing tests.
      </div>
    )
  }

  const rows: FailureRow[] = failingSuites.flatMap((suite, i) => {
    const tests = queries[i]?.data ?? []
    return tests.map((t) => ({
      key: `${suite.project_id}-${suite.build_id}-${t.history_id}`,
      testName: t.test_name,
      suiteSlug: suite.slug,
      suiteHref: `/projects/${suite.project_id}/reports/${suite.build_number}`,
      flaky: t.flaky,
      newFailed: t.new_failed,
      known: t.known,
      errorMessage: t.error_message,
      errorMessageFirstLine: t.error_message?.split('\n')[0] ?? '',
    }))
  })

  return (
    <div className="mt-3 space-y-2" data-testid="run-failures">
      {rows.map((row) => (
        <div
          key={row.key}
          className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t pt-2 text-sm first:border-t-0 first:pt-0"
        >
          <span className="min-w-0 flex-1 truncate font-medium" title={row.testName}>
            {row.testName}
          </span>
          <NavLink to={row.suiteHref} className="text-muted-foreground text-xs hover:underline">
            {row.suiteSlug}
          </NavLink>
          <FailureBadges flaky={row.flaky} newFailed={row.newFailed} known={row.known} />
          <span
            className="text-muted-foreground max-w-xs truncate text-xs"
            title={row.errorMessage}
          >
            {row.errorMessageFirstLine}
          </span>
        </div>
      ))}
    </div>
  )
}
