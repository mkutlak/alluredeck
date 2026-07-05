import { describe, it, expect, vi } from 'vitest'
import { flakyImpactOptions } from './analytics'
import { queryKeys } from '@/lib/query-keys'

vi.mock('@/api/analytics', () => ({
  fetchFlakyImpact: vi.fn(),
}))

describe('flakyImpactOptions', () => {
  it('returns the flaky-impact queryKey', () => {
    const opts = flakyImpactOptions('proj', { builds: 20, limit: 10, branch: 'main' })
    expect(opts.queryKey).toEqual(queryKeys.flakyImpact('proj', 20, 10, 'main'))
  })

  it('defaults to no params', () => {
    const opts = flakyImpactOptions('proj')
    expect(opts.queryKey).toEqual(queryKeys.flakyImpact('proj', undefined, undefined, undefined))
  })

  it('has staleTime of 60000', () => {
    expect(flakyImpactOptions('proj').staleTime).toBe(60_000)
  })

  it('has a queryFn', () => {
    expect(typeof flakyImpactOptions('proj').queryFn).toBe('function')
  })
})
