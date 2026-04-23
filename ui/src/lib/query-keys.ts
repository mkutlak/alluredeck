import type { QueryClient } from '@tanstack/react-query'

export const queryKeys = {
  projects: ['projects'] as const,
  dashboard: () => ['dashboard'] as const,
  search: (query: string) => ['search', query] as const,
  // Project-scoped
  reportHistory: (pid: string, page?: number, branch?: string, perPage?: number) =>
    ['report-history', pid, page ?? undefined, branch ?? undefined, perPage ?? undefined] as const,
  reportCategories: (pid: string) => ['report-categories', pid] as const,
  reportCategoriesLatest: (pid: string) => ['report-categories', pid, 'latest'] as const,
  reportEnvironment: (pid: string) => ['report-environment', pid] as const,
  reportStability: (pid: string) => ['report-stability', pid] as const,
  reportKnownFailures: (pid: string) => ['report-known-failures', pid] as const,
  reportTimeline: (pid: string) => ['report-timeline', pid] as const,
  projectTimeline: (pid: string, branch?: string, from?: string, to?: string, limit?: number) =>
    ['project-timeline', pid, branch, from, to, limit] as const,
  reportHistoryAnalytics: (pid: string, branch?: string) =>
    branch != null
      ? (['report-history-analytics', pid, branch] as const)
      : (['report-history-analytics', pid] as const),
  lowPerforming: (pid: string, sort?: string, branch?: string) =>
    sort !== undefined || branch !== undefined
      ? (['low-performing-tests', pid, sort ?? undefined, branch ?? undefined] as const)
      : (['low-performing-tests', pid] as const),
  knownIssues: (pid: string, showResolved?: boolean) =>
    ['known-issues', pid, showResolved ?? undefined] as const,
  jobStatus: (pid: string, jid: string) => ['job-status', pid, jid] as const,
  buildComparison: (pid: string, a: number, b: number) => ['build-comparison', pid, a, b] as const,
  adminJobs: (page?: number, perPage?: number) =>
    ['admin-jobs', page ?? undefined, perPage ?? undefined] as const,
  adminResults: ['admin-results'] as const,
  apiKeys: ['api-keys'] as const,
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
  topErrors: (projectId: string, builds: number, branch?: string) =>
    branch != null
      ? (['top-errors', projectId, builds, branch] as const)
      : (['top-errors', projectId, builds] as const),
  suitePassRates: (projectId: string, builds: number, branch?: string) =>
    branch != null
      ? (['suite-pass-rates', projectId, builds, branch] as const)
      : (['suite-pass-rates', projectId, builds] as const),
  labelBreakdown: (projectId: string, name: string, builds: number, branch?: string) =>
    branch != null
      ? (['label-breakdown', projectId, name, builds, branch] as const)
      : (['label-breakdown', projectId, name, builds] as const),
  trends: (pid: string, builds: number, branch?: string) =>
    branch != null
      ? (['trends', pid, builds, branch] as const)
      : (['trends', pid, builds] as const),
  attachments: (projectId: string, reportId: string, mimeType?: string) =>
    mimeType != null
      ? (['attachments', projectId, reportId, mimeType] as const)
      : (['attachments', projectId, reportId] as const),
  defects: (projectId: string, filters?: Record<string, unknown>) =>
    filters != null
      ? (['defects', projectId, filters] as const)
      : (['defects', projectId] as const),
  buildDefects: (projectId: string, buildId: number, filters?: Record<string, unknown>) =>
    filters != null
      ? (['defects', 'build', projectId, buildId, filters] as const)
      : (['defects', 'build', projectId, buildId] as const),
  defectDetail: (defectId: string) => ['defects', 'detail', defectId] as const,
  defectProjectSummary: (projectId: string) => ['defects', 'summary', projectId] as const,
  defectBuildSummary: (projectId: string, buildId: number) =>
    ['defects', 'buildSummary', projectId, buildId] as const,
  webhooks: (projectId: string) => ['webhooks', projectId] as const,
  webhookDeliveries: (projectId: string, webhookId: string, page?: number) =>
    ['webhook-deliveries', projectId, webhookId, ...(page !== undefined ? [page] : [])] as const,
  pipelineRuns: (pid: string, page?: number, branch?: string) =>
    branch != null
      ? (['pipeline-runs', pid, page ?? undefined, branch] as const)
      : (['pipeline-runs', pid, page ?? undefined] as const),
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
    queryKeys.attachments(projectId, 'latest'),
    queryKeys.trends(projectId, 100),
    queryKeys.pipelineRuns(projectId),
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
