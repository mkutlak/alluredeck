import { useState } from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import { NavLink } from 'react-router'
import { ChevronDown, ChevronRight } from 'lucide-react'

import { buildFailedTestsOptions } from '@/lib/queries'
import { getConfig } from '@/api/system'
import { FailureBadges } from './FailureBadges'
import { FailureSummaryPanel } from './FailureSummaryPanel'
import type { PipelineRun } from '@/types/api'

export interface RunFailuresProps {
  run: PipelineRun
}

interface FailureRow {
  key: string
  testName: string
  suiteSlug: string
  suiteHref: string
  projectId: number
  buildId: number
  historyId: string
  flaky: boolean
  retries: number
  newFailed: boolean
  known: boolean
  errorMessage: string
  errorMessageFirstLine: string
}

export function RunFailures({ run }: RunFailuresProps) {
  const failingSuites = run.suites.filter((s) => s.failed > 0)
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set())

  // Read once here (shares the app-wide `['config']` cache entry with
  // AppSidebar/LoginPage/etc.) so the "AI summary" toggle can be hidden
  // entirely when the feature is off, instead of expanding to nothing.
  const { data: configResp } = useQuery({ queryKey: ['config'], queryFn: getConfig })
  const llmEnabled = configResp?.data.llm_enabled === true

  const queries = useQueries({
    queries: failingSuites.map((s) => buildFailedTestsOptions(s.project_id, s.build_id)),
  })

  if (failingSuites.length === 0) {
    return null
  }

  function toggleExpanded(key: string) {
    setExpandedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  return (
    <div className="mt-3 space-y-2" data-testid="run-failures">
      {failingSuites.flatMap((suite, i) => {
        const query = queries[i]
        const suiteKey = `${suite.project_id}-${suite.build_id}`

        if (!query || query.isLoading) {
          return [
            <div
              key={`${suiteKey}-loading`}
              className="text-muted-foreground border-t pt-2 text-sm first:border-t-0 first:pt-0"
            >
              Loading failures for {suite.slug}…
            </div>,
          ]
        }

        if (query.isError) {
          return [
            <div
              key={`${suiteKey}-error`}
              className="text-destructive border-t pt-2 text-sm first:border-t-0 first:pt-0"
            >
              Failed to load failing tests for {suite.slug}.
            </div>,
          ]
        }

        const tests = query.data ?? []
        const rows: FailureRow[] = tests.map((t) => ({
          key: `${suiteKey}-${t.history_id}`,
          testName: t.test_name,
          suiteSlug: suite.slug,
          suiteHref: `/projects/${suite.project_id}/reports/${suite.build_number}`,
          projectId: suite.project_id,
          buildId: suite.build_id,
          historyId: t.history_id,
          flaky: t.flaky,
          retries: t.retries,
          newFailed: t.new_failed,
          known: t.known,
          errorMessage: t.error_message,
          errorMessageFirstLine: t.error_message?.split('\n')[0] ?? '',
        }))

        return rows.map((row) => {
          const isExpanded = expandedKeys.has(row.key)

          return (
            <div key={row.key} className="border-t pt-2 first:border-t-0 first:pt-0">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                <span className="min-w-0 flex-1 truncate font-medium" title={row.testName}>
                  {row.testName}
                </span>
                <NavLink
                  to={row.suiteHref}
                  className="text-muted-foreground text-xs hover:underline"
                >
                  {row.suiteSlug}
                </NavLink>
                <FailureBadges
                  flaky={row.flaky}
                  retries={row.retries}
                  newFailed={row.newFailed}
                  known={row.known}
                />
                <span
                  className="text-muted-foreground max-w-xs truncate text-xs"
                  title={row.errorMessage}
                >
                  {row.errorMessageFirstLine}
                </span>
                {llmEnabled && (
                  <button
                    type="button"
                    onClick={() => toggleExpanded(row.key)}
                    aria-expanded={isExpanded}
                    aria-label={`Toggle AI failure summary for ${row.testName}`}
                    className="text-muted-foreground hover:text-foreground flex items-center gap-0.5 text-xs"
                  >
                    {isExpanded ? (
                      <ChevronDown size={14} className="shrink-0" />
                    ) : (
                      <ChevronRight size={14} className="shrink-0" />
                    )}
                    AI summary
                  </button>
                )}
              </div>

              {llmEnabled && isExpanded && (
                <FailureSummaryPanel
                  projectId={row.projectId}
                  buildId={row.buildId}
                  historyId={row.historyId}
                  open={isExpanded}
                />
              )}
            </div>
          )
        })
      })}
    </div>
  )
}
