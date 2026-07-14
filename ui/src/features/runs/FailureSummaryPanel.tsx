import { useQuery } from '@tanstack/react-query'
import { NavLink } from 'react-router'
import { AlertTriangle } from 'lucide-react'

import { getConfig } from '@/api/system'
import { failureSummaryOptions } from '@/lib/queries/failureSummary'
import { CardState } from '@/components/ui/CardState'
import { Badge } from '@/components/ui/badge'
import { Markdown } from '@/components/ui/Markdown'
import { ACCENT_BADGE_CLASSES, INFO_BADGE_CLASSES, NEUTRAL_BADGE_CLASSES } from '@/lib/status-colors'

export interface FailureSummaryPanelProps {
  projectId: number
  buildId: number
  historyId: string
  /** Whether the parent disclosure is currently expanded — gates the query. */
  open: boolean
}

/**
 * Opt-in, in-product LLM failure hypothesis panel (B1 Phase B).
 * Reads `/config` for the `llm_enabled` flag and renders nothing when the
 * feature is disabled server-side.
 */
export function FailureSummaryPanel({
  projectId,
  buildId,
  historyId,
  open,
}: FailureSummaryPanelProps) {
  const { data: configResp } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
    staleTime: 5 * 60_000,
  })
  const llmEnabled = configResp?.data.llm_enabled === true

  const query = useQuery(failureSummaryOptions(projectId, buildId, historyId, llmEnabled, open))

  if (!llmEnabled || !open) {
    return null
  }

  // Defensive: an authoritative "disabled" response from the server wins
  // over the locally cached config flag.
  if (query.data?.enabled === false) {
    return null
  }

  const summary = query.data?.summary
  const evidence = summary?.evidence ?? []

  return (
    <div
      className="bg-muted/30 mt-2 rounded-md border p-3"
      data-testid="failure-summary-panel"
    >
      <CardState
        isLoading={query.isLoading}
        isError={query.isError}
        error={query.error}
        isEmpty={false}
        refetch={() => void query.refetch()}
        skeletonRows={2}
      >
        {!summary ? (
          <div
            className="text-muted-foreground flex items-center gap-2 text-xs"
            data-testid="failure-summary-soft-error"
          >
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            {query.data?.error ?? 'AI summary is unavailable for this failure.'}
          </div>
        ) : (
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge className={INFO_BADGE_CLASSES}>AI hypothesis</Badge>
              {summary.category && (
                <Badge className={ACCENT_BADGE_CLASSES}>{summary.category}</Badge>
              )}
              {summary.confidence && (
                <Badge className={NEUTRAL_BADGE_CLASSES}>{summary.confidence} confidence</Badge>
              )}
            </div>

            <Markdown text={summary.hypothesis} />

            {evidence.length > 0 && (
              <ul className="text-muted-foreground list-disc space-y-0.5 pl-5 text-xs">
                {evidence.map((item, i) => (
                  <li key={i}>{item}</li>
                ))}
              </ul>
            )}

            {query.data?.last_good && (
              <NavLink
                to={`/projects/${projectId}/reports/${query.data.last_good.build_number}`}
                className="text-muted-foreground block text-xs hover:underline"
              >
                Last passed: build #{query.data.last_good.build_number} (
                {query.data.last_good.builds_since} builds ago)
              </NavLink>
            )}

            {query.data?.disclaimer && (
              <p className="text-muted-foreground text-[10px] italic">{query.data.disclaimer}</p>
            )}
          </div>
        )}
      </CardState>
    </div>
  )
}
