import { queryOptions } from '@tanstack/react-query'

import { fetchFlakyImpact } from '@/api/analytics'
import { queryKeys } from '@/lib/query-keys'

export interface FlakyImpactParams {
  builds?: number
  limit?: number
  branch?: string
}

export function flakyImpactOptions(projectId: string, params: FlakyImpactParams = {}) {
  const { builds, limit, branch } = params
  return queryOptions({
    queryKey: queryKeys.flakyImpact(projectId, builds, limit, branch),
    queryFn: () => fetchFlakyImpact(projectId, builds, limit, branch),
    staleTime: 60_000,
  })
}
