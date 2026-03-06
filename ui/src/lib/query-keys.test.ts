import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { QueryClient } from '@tanstack/react-query'
import { queryKeys, invalidateProjectQueries, removeProjectQueries } from './query-keys'

function makeMockQueryClient() {
  return {
    invalidateQueries: vi.fn(() => Promise.resolve()),
    removeQueries: vi.fn(),
  } as unknown as QueryClient
}

describe('queryKeys', () => {
  it('projects is a static key', () => {
    expect(queryKeys.projects).toEqual(['projects'])
  })

  it('dashboard is a static key', () => {
    expect(queryKeys.dashboard).toEqual(['dashboard'])
  })

  it('reportHistory without page returns prefix key', () => {
    expect(queryKeys.reportHistory('p1')).toEqual(['report-history', 'p1'])
  })

  it('reportHistory with page includes page number', () => {
    expect(queryKeys.reportHistory('p1', 3)).toEqual(['report-history', 'p1', 3])
  })

  it('reportCategories', () => {
    expect(queryKeys.reportCategories('p1')).toEqual(['report-categories', 'p1'])
  })

  it('reportCategoriesLatest', () => {
    expect(queryKeys.reportCategoriesLatest('p1')).toEqual(['report-categories', 'p1', 'latest'])
  })

  it('reportEnvironment', () => {
    expect(queryKeys.reportEnvironment('p1')).toEqual(['report-environment', 'p1'])
  })

  it('reportStability', () => {
    expect(queryKeys.reportStability('p1')).toEqual(['report-stability', 'p1'])
  })

  it('reportKnownFailures', () => {
    expect(queryKeys.reportKnownFailures('p1')).toEqual(['report-known-failures', 'p1'])
  })

  it('reportTimeline', () => {
    expect(queryKeys.reportTimeline('p1')).toEqual(['report-timeline', 'p1'])
  })

  it('reportHistoryAnalytics', () => {
    expect(queryKeys.reportHistoryAnalytics('p1')).toEqual(['report-history-analytics', 'p1'])
  })

  it('lowPerforming', () => {
    expect(queryKeys.lowPerforming('p1')).toEqual(['low-performing-tests', 'p1'])
  })

  it('knownIssues', () => {
    expect(queryKeys.knownIssues('p1')).toEqual(['known-issues', 'p1'])
  })

  it('jobStatus', () => {
    expect(queryKeys.jobStatus('p1', 'j42')).toEqual(['job-status', 'p1', 'j42'])
  })
})

describe('invalidateProjectQueries', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('invalidates dashboard', async () => {
    const qc = makeMockQueryClient()
    await invalidateProjectQueries(qc, 'proj')
    expect(qc.invalidateQueries).toHaveBeenCalledWith({ queryKey: queryKeys.dashboard })
  })

  it('invalidates all project-scoped query prefixes', async () => {
    const qc = makeMockQueryClient()
    await invalidateProjectQueries(qc, 'proj')

    const expectedKeys = [
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
    ]

    for (const key of expectedKeys) {
      expect(qc.invalidateQueries).toHaveBeenCalledWith({ queryKey: key })
    }
  })

  it('makes 11 total invalidation calls (1 dashboard + 10 project-scoped)', async () => {
    const qc = makeMockQueryClient()
    await invalidateProjectQueries(qc, 'proj')
    expect(qc.invalidateQueries).toHaveBeenCalledTimes(11)
  })
})

describe('removeProjectQueries', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('removes all project-scoped keys', () => {
    const qc = makeMockQueryClient()
    removeProjectQueries(qc, 'proj')

    const expectedKeys = [
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
    ]

    for (const key of expectedKeys) {
      expect(qc.removeQueries).toHaveBeenCalledWith({ queryKey: key })
    }
  })

  it('does not remove global keys (dashboard, projects)', () => {
    const qc = makeMockQueryClient()
    removeProjectQueries(qc, 'proj')
    expect(qc.removeQueries).not.toHaveBeenCalledWith({ queryKey: queryKeys.dashboard })
    expect(qc.removeQueries).not.toHaveBeenCalledWith({ queryKey: queryKeys.projects })
  })

  it('makes exactly 10 removeQueries calls', () => {
    const qc = makeMockQueryClient()
    removeProjectQueries(qc, 'proj')
    expect(qc.removeQueries).toHaveBeenCalledTimes(10)
  })
})
