import { describe, it, expect, vi } from 'vitest'
import type { QueryClient } from '@tanstack/react-query'
import { queryKeys, invalidateProjectQueries, removeProjectQueries } from './query-keys'

function makeMockQueryClient() {
  return {
    invalidateQueries: vi.fn(() => Promise.resolve()),
    removeQueries: vi.fn(),
  } as unknown as QueryClient
}

const projectScopedKeys = [
  queryKeys.reportHistory('proj'),
  queryKeys.reportCategories('proj'),
  queryKeys.reportCategoriesLatest('proj'),
  queryKeys.reportEnvironment('proj'),
  queryKeys.reportStability('proj'),
  queryKeys.reportKnownFailures('proj'),
  queryKeys.reportTimeline('proj'),
  queryKeys.reportHistoryAnalytics('proj'),
  queryKeys.lowPerforming('proj'),
  queryKeys.knownIssues('proj'),
  queryKeys.attachments('proj', 'latest'),
  queryKeys.trends('proj', 100),
  queryKeys.pipelineRuns('proj'),
]

describe('queryKeys', () => {
  it('runsFeed sorts groupIds for a stable key regardless of selection order', () => {
    expect(queryKeys.runsFeed(1, undefined, [7, 3])).toEqual(
      queryKeys.runsFeed(1, undefined, [3, 7]),
    )
  })

  it('runsFeed treats an empty groupIds array as no filter', () => {
    expect(queryKeys.runsFeed(1, undefined, [])).toEqual(queryKeys.runsFeed(1))
  })
})

describe('invalidateProjectQueries', () => {
  it('invalidates the dashboard and every project-scoped key', async () => {
    const qc = makeMockQueryClient()
    await invalidateProjectQueries(qc, 'proj')

    expect(qc.invalidateQueries).toHaveBeenCalledWith({ queryKey: queryKeys.dashboard() })
    for (const key of projectScopedKeys) {
      expect(qc.invalidateQueries).toHaveBeenCalledWith({ queryKey: key })
    }
  })
})

describe('removeProjectQueries', () => {
  it('removes every project-scoped key but no global key (dashboard, projects)', () => {
    const qc = makeMockQueryClient()
    removeProjectQueries(qc, 'proj')

    for (const key of projectScopedKeys) {
      expect(qc.removeQueries).toHaveBeenCalledWith({ queryKey: key })
    }
    expect(qc.removeQueries).not.toHaveBeenCalledWith({ queryKey: queryKeys.dashboard() })
    expect(qc.removeQueries).not.toHaveBeenCalledWith({ queryKey: queryKeys.projects })
  })
})
