import { queryOptions } from '@tanstack/react-query'

import { fetchPipelineRuns } from '@/api/pipeline'
import { queryKeys } from '@/lib/query-keys'

export function pipelineRunsOptions(projectId: string, page: number, branch?: string) {
  return queryOptions({
    queryKey: queryKeys.pipelineRuns(projectId, page, branch),
    queryFn: () => fetchPipelineRuns(projectId, page, undefined, branch),
    staleTime: 5_000,
  })
}
