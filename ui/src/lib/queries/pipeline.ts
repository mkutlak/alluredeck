import { queryOptions, type QueryStatus } from '@tanstack/react-query'

import { ApiError } from '@/api/client'
import {
  fetchPipelineRun,
  fetchPipelineRuns,
  fetchRunFailures,
  fetchRunsFeed,
} from '@/api/pipeline'
import { fetchBuildFailedTests } from '@/api/builds'
import { queryKeys } from '@/lib/query-keys'
import type { PipelineRun } from '@/types/api'

/** How often a view of a run that is still uploading looks for the suites that arrived since. */
export const AWAITING_POLL_MS = 30_000

/**
 * `refetchInterval` for any query that returns runs (one or a list): poll while a run
 * is still awaiting suites, stop once none is. TanStack skips polling in a hidden tab. A query
 * in error stops too (an outage, a deleted run): a refetch that succeeds starts it again.
 */
export function pollWhileAwaiting(query: {
  state: { status: QueryStatus; data: { data: PipelineRun | PipelineRun[] } | undefined }
}): number | false {
  if (query.state.status === 'error') return false
  const runs = query.state.data?.data
  const awaiting = [runs ?? []].flat().some((run) => (run.aggregate.awaiting_suites ?? 0) > 0)
  return awaiting ? AWAITING_POLL_MS : false
}

export function pipelineRunsOptions(projectId: string, page: number, branch?: string) {
  return queryOptions({
    queryKey: queryKeys.pipelineRuns(projectId, page, branch),
    queryFn: () => fetchPipelineRuns(projectId, page, undefined, branch),
    staleTime: 5_000,
    refetchInterval: pollWhileAwaiting,
  })
}

export function runsFeedOptions(page: number, branch?: string, groupIds?: number[]) {
  return queryOptions({
    queryKey: queryKeys.runsFeed(page, branch, groupIds),
    queryFn: () => fetchRunsFeed(page, undefined, branch, groupIds),
    staleTime: 15_000,
    refetchInterval: pollWhileAwaiting,
  })
}

/** One run of a group, for its page. A 404 settles at once: the run does not exist. */
export function pipelineRunOptions(groupId: string, runKey: string) {
  return queryOptions({
    queryKey: queryKeys.pipelineRun(groupId, runKey),
    queryFn: () => fetchPipelineRun(groupId, runKey),
    staleTime: 15_000,
    refetchInterval: pollWhileAwaiting,
    retry: (failures, error) =>
      !(error instanceof ApiError && error.response?.status === 404) && failures < 1,
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
 *
 * `failingBuilds` names the builds that fail the run (`failingBuildIds`). It is part of the
 * key, so the list is fetched again when a failing suite arrives or is replaced while a
 * run that is still uploading is polled, instead of waiting out its cache.
 */
export function runFailuresOptions(groupProjectId: number, runKey: string, failingBuilds: string) {
  return queryOptions({
    queryKey: queryKeys.runFailures(groupProjectId, runKey, failingBuilds),
    queryFn: () => fetchRunFailures(groupProjectId, runKey),
    staleTime: 5 * 60_000,
  })
}
