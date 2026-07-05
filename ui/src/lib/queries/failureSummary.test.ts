import { describe, it, expect, vi } from 'vitest'
import { failureSummaryOptions } from './failureSummary'
import { queryKeys } from '@/lib/query-keys'

vi.mock('@/api/failures', () => ({
  fetchFailureSummary: vi.fn(),
}))

describe('failureSummaryOptions', () => {
  it('returns the failureSummary queryKey', () => {
    const opts = failureSummaryOptions(1, 42, 'h1', true, true)
    expect(opts.queryKey).toEqual(queryKeys.failureSummary(42, 'h1'))
  })

  it('has staleTime of 60 seconds', () => {
    expect(failureSummaryOptions(1, 42, 'h1', true, true).staleTime).toBe(60_000)
  })

  it('is enabled when llmEnabled and open are both true and ids are present', () => {
    expect(failureSummaryOptions(1, 42, 'h1', true, true).enabled).toBe(true)
  })

  it('is disabled when llmEnabled is false', () => {
    expect(failureSummaryOptions(1, 42, 'h1', false, true).enabled).toBe(false)
  })

  it('is disabled when open is false', () => {
    expect(failureSummaryOptions(1, 42, 'h1', true, false).enabled).toBe(false)
  })

  it('is disabled when buildId is 0', () => {
    expect(failureSummaryOptions(1, 0, 'h1', true, true).enabled).toBe(false)
  })

  it('is disabled when historyId is empty', () => {
    expect(failureSummaryOptions(1, 42, '', true, true).enabled).toBe(false)
  })
})
