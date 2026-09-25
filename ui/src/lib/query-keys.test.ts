import { describe, it, expect } from 'vitest'
import { QueryClient, type QueryKey } from '@tanstack/react-query'
import { queryKeys, invalidateProjectQueries, removeProjectQueries } from './query-keys'

// Keys shaped the way feature code builds them (page, branch, filters set), so
// the helpers are exercised against TanStack's real partial key matching.
function liveProjectKeys(pid: string): QueryKey[] {
  return [
    queryKeys.reportHistory(pid, 2, 'main', 20),
    queryKeys.reportHistory(pid, 1),
    queryKeys.reportCategories(pid),
    queryKeys.reportCategoriesLatest(pid),
    queryKeys.reportEnvironment(pid),
    queryKeys.reportStability(pid),
    queryKeys.reportKnownFailures(pid),
    queryKeys.reportTimeline(pid),
    queryKeys.reportHistoryAnalytics(pid, 'main'),
    queryKeys.lowPerforming(pid, 'duration', 'main'),
    queryKeys.knownIssues(pid, false),
    queryKeys.attachments(pid, 'latest', 'failed'),
    queryKeys.attachments(pid, '42'),
    queryKeys.trends(pid, 100, 'main'),
    queryKeys.pipelineRuns(pid, 1),
    queryKeys.pipelineRuns(pid, 2, 'main'),
  ]
}

const globalKeys: QueryKey[] = [queryKeys.projects, queryKeys.runsFeed(1), queryKeys.apiKeys]

function seededClient(): QueryClient {
  const qc = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } })
  const keys = [
    ...liveProjectKeys('7'),
    ...liveProjectKeys('8'),
    queryKeys.dashboard(),
    ...globalKeys,
  ]
  for (const key of keys) qc.setQueryData(key, { seeded: true })
  return qc
}

const isInvalidated = (qc: QueryClient, key: QueryKey) =>
  qc.getQueryState(key)?.isInvalidated === true
const isCached = (qc: QueryClient, key: QueryKey) =>
  qc.getQueryCache().find({ queryKey: key, exact: true }) !== undefined

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
  it('invalidates the dashboard and every live query of the project', async () => {
    const qc = seededClient()
    await invalidateProjectQueries(qc, '7')

    expect(liveProjectKeys('7').filter((key) => !isInvalidated(qc, key))).toEqual([])
    expect(isInvalidated(qc, queryKeys.dashboard())).toBe(true)
  })

  it('leaves other projects and global queries untouched', async () => {
    const qc = seededClient()
    await invalidateProjectQueries(qc, '7')

    const untouched = [...liveProjectKeys('8'), ...globalKeys]
    expect(untouched.filter((key) => isInvalidated(qc, key))).toEqual([])
  })
})

describe('removeProjectQueries', () => {
  it('removes every live query of the project', () => {
    const qc = seededClient()
    removeProjectQueries(qc, '7')

    expect(liveProjectKeys('7').filter((key) => isCached(qc, key))).toEqual([])
  })

  it('keeps other projects, the dashboard and global queries', () => {
    const qc = seededClient()
    removeProjectQueries(qc, '7')

    const kept = [...liveProjectKeys('8'), queryKeys.dashboard(), ...globalKeys]
    expect(kept.filter((key) => !isCached(qc, key))).toEqual([])
  })
})
