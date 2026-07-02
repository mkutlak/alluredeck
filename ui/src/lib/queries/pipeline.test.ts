import { describe, it, expect, vi } from 'vitest'
import { pipelineRunsOptions, runsFeedOptions, buildFailedTestsOptions } from './pipeline'
import { queryKeys } from '@/lib/query-keys'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/builds', () => ({
  fetchBuildFailedTests: vi.fn(),
}))

describe('pipelineRunsOptions', () => {
  it('returns the project-scoped queryKey', () => {
    const opts = pipelineRunsOptions('proj', 1)
    expect(opts.queryKey).toEqual(queryKeys.pipelineRuns('proj', 1, undefined))
  })

  it('has staleTime of 5000', () => {
    expect(pipelineRunsOptions('proj', 1).staleTime).toBe(5_000)
  })
})

describe('runsFeedOptions', () => {
  it('returns the runsFeed queryKey', () => {
    const opts = runsFeedOptions(2, 'main', [1, 2])
    expect(opts.queryKey).toEqual(queryKeys.runsFeed(2, 'main', [1, 2]))
  })

  it('has staleTime of 15000 (no polling)', () => {
    expect(runsFeedOptions(1).staleTime).toBe(15_000)
  })

  it('has a queryFn', () => {
    expect(typeof runsFeedOptions(1).queryFn).toBe('function')
  })
})

describe('buildFailedTestsOptions', () => {
  it('returns the buildFailedTests queryKey', () => {
    const opts = buildFailedTestsOptions(5, 42)
    expect(opts.queryKey).toEqual(queryKeys.buildFailedTests(5, 42))
  })

  it('has staleTime of 5 minutes', () => {
    expect(buildFailedTestsOptions(5, 42).staleTime).toBe(5 * 60_000)
  })
})
