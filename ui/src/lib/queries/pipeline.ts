import { queryOptions } from '@tanstack/react-query'

import { fetchPipelineRuns, fetchRunFailures, fetchRunsFeed } from '@/api/pipeline'
import { fetchBuildFailedTests } from '@/api/builds'
import { queryKeys } from '@/lib/query-keys'

export function pipelineRunsOptions(projectId: string, page: number, branch?: string) {
  return queryOptions({
    queryKey: queryKeys.pipelineRuns(projectId, page, branch),
    queryFn: () => fetchPipelineRuns(projectId, page, undefined, branch),
    staleTime: 5_000,
  })
}

export function runsFeedOptions(page: number, branch?: string, groupIds?: number[]) {
  return queryOptions({
    queryKey: queryKeys.runsFeed(page, branch, groupIds),
    queryFn: () => fetchRunsFeed(page, undefined, branch, groupIds),
    staleTime: 15_000,
  })
}

export function buildFailedTestsOptions(projectId: number, buildId: number) {
  return queryOptions({
    queryKey: queryKeys.buildFailedTests(projectId, buildId),
    queryFn: () => fetchBuildFailedTests(projectId, buildId),
    staleTime: 5 * 60_000,
  })
}

/**
 * One request covers a whole run's failures. Callers pass `enabled: false`
 * until the run is expanded, so a feed of collapsed runs fetches nothing.
 */
export function runFailuresOptions(groupProjectId: number, runKey: string) {
  return queryOptions({
    queryKey: queryKeys.runFailures(groupProjectId, runKey),
    queryFn: () => fetchRunFailures(groupProjectId, runKey),
    staleTime: 5 * 60_000,
  })
}
