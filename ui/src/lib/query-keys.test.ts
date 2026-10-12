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
    queryKeys.lowPerforming(pid, 'duration', 'main'),
    queryKeys.knownIssues(pid, false),
    queryKeys.attachments(pid, 'latest', 'failed'),
    queryKeys.attachments(pid, '42'),
    queryKeys.trends(pid, 100, 'main'),
    queryKeys.pipelineRuns(pid, 1),
    queryKeys.pipelineRuns(pid, 2, 'main'),
    // Families the project id sits in a later slot of, or as a number.
    queryKeys.defects(pid, { status: 'open' }),
    queryKeys.buildDefects(pid, 3),
    queryKeys.defectTests(pid, 'd1'),
    queryKeys.defectProjectSummary(pid),
    queryKeys.defectBuildSummary(pid, 3),
    queryKeys.branches.list(pid),
    queryKeys.projectTimeline(pid, 'main'),
    queryKeys.topErrors(pid, 10),
    queryKeys.suitePassRates(pid, 10),
    queryKeys.labelBreakdown(pid, 'suite', 10),
    queryKeys.flakyImpact(pid, 10),
    queryKeys.buildComparison(pid, 1, 2),
    queryKeys.tests.history(pid, 'h1'),
    queryKeys.webhooks(pid),
    queryKeys.buildFailedTests(Number(pid), 3),
    queryKeys.runFailures(Number(pid), 'run-1', '3'),
    queryKeys.proposals('flaky', Number(pid)),
    ['project', pid],
  ]
}

const globalKeys: QueryKey[] = [queryKeys.projects, queryKeys.apiKeys]
const feedKey = queryKeys.runsFeed(1)

function seededClient(): QueryClient {
  const qc = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } })
  const keys = [
    ...liveProjectKeys('7'),
    ...liveProjectKeys('8'),
    queryKeys.dashboard(),
    ...globalKeys,
    feedKey,
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

  it('invalidates the runs feed, which lists the project runs', async () => {
    const qc = seededClient()
    await invalidateProjectQueries(qc, '7')

    expect(isInvalidated(qc, feedKey)).toBe(true)
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

  it('keeps the runs feed but marks it stale, as it lists the removed project runs', () => {
    const qc = seededClient()
    removeProjectQueries(qc, '7')

    expect(isCached(qc, feedKey)).toBe(true)
    expect(isInvalidated(qc, feedKey)).toBe(true)
  })
})
