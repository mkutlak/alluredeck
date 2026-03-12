import type { QueryClient } from '@tanstack/react-query'

export const queryKeys = {
  projects: ['projects'] as const,
  tags: ['tags'] as const,
  dashboard: (tag?: string) =>
    tag !== undefined ? (['dashboard', tag] as const) : (['dashboard'] as const),
  search: (query: string) => ['search', query] as const,
  // Project-scoped
  reportHistory: (pid: string, page?: number, branch?: string) =>
    page != null
      ? branch != null
        ? (['report-history', pid, page, branch] as const)
        : (['report-history', pid, page] as const)
      : (['report-history', pid] as const),
  reportCategories: (pid: string) => ['report-categories', pid] as const,
  reportCategoriesLatest: (pid: string) => ['report-categories', pid, 'latest'] as const,
  reportEnvironment: (pid: string) => ['report-environment', pid] as const,
  reportStability: (pid: string) => ['report-stability', pid] as const,
  reportKnownFailures: (pid: string) => ['report-known-failures', pid] as const,
  reportTimeline: (pid: string) => ['report-timeline', pid] as const,
  reportHistoryAnalytics: (pid: string) => ['report-history-analytics', pid] as const,
  lowPerforming: (pid: string, sort?: string) =>
    sort !== undefined
      ? (['low-performing-tests', pid, sort] as const)
      : (['low-performing-tests', pid] as const),
  knownIssues: (pid: string) => ['known-issues', pid] as const,
  jobStatus: (pid: string, jid: string) => ['job-status', pid, jid] as const,
  buildComparison: (pid: string, a: number, b: number) => ['build-comparison', pid, a, b] as const,
  adminJobs: ['admin-jobs'] as const,
  adminResults: ['admin-results'] as const,
  branches: {
    list: (projectId: string) => ['branches', projectId] as const,
  },
  tests: {
    history: (projectId: string, historyId: string, branch?: string) =>
      branch != null
        ? (['test-history', projectId, historyId, branch] as const)
        : (['test-history', projectId, historyId] as const),
  },
  // Phase 8 — PostgreSQL analytics dashboards
  topErrors: (projectId: string, builds: number) => ['top-errors', projectId, builds] as const,
  suitePassRates: (projectId: string, builds: number) =>
    ['suite-pass-rates', projectId, builds] as const,
  labelBreakdown: (projectId: string, name: string, builds: number) =>
    ['label-breakdown', projectId, name, builds] as const,
}

function projectScopedKeys(projectId: string) {
  return [
    queryKeys.reportHistory(projectId),
    queryKeys.reportCategories(projectId),
    queryKeys.reportCategoriesLatest(projectId),
    queryKeys.reportEnvironment(projectId),
    queryKeys.reportStability(projectId),
    queryKeys.reportKnownFailures(projectId),
    queryKeys.reportTimeline(projectId),
    queryKeys.reportHistoryAnalytics(projectId),
    queryKeys.lowPerforming(projectId),
    queryKeys.knownIssues(projectId),
  ]
}

/**
 * Invalidates dashboard + all project-scoped queries for the given project.
 * Use in mutation onSuccess when project data changes (report generated,
 * deleted, history cleaned, known issue changed, etc.).
 */
export async function invalidateProjectQueries(
  queryClient: QueryClient,
  projectId: string,
): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.dashboard() }),
    ...projectScopedKeys(projectId).map((key) => queryClient.invalidateQueries({ queryKey: key })),
  ])
}

/**
 * Removes all project-scoped queries from the cache.
 * Use when a project is deleted to prevent stale data from briefly rendering
 * if the user navigates back via browser history.
 */
export function removeProjectQueries(queryClient: QueryClient, projectId: string): void {
  for (const key of projectScopedKeys(projectId)) {
    queryClient.removeQueries({ queryKey: key })
  }
}
