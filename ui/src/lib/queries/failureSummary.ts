import { queryOptions } from '@tanstack/react-query'

import { fetchFailureSummary } from '@/api/failures'
import { queryKeys } from '@/lib/query-keys'

export function failureSummaryOptions(
  projectId: number,
  buildId: number,
  historyId: string,
  llmEnabled: boolean,
  open: boolean,
) {
  return queryOptions({
    queryKey: queryKeys.failureSummary(buildId, historyId),
    queryFn: () => fetchFailureSummary(projectId, buildId, historyId),
    enabled: llmEnabled && open && !!buildId && !!historyId,
    staleTime: 60_000,
  })
}
