import { describe, it, expect, vi } from 'vitest'
import { dashboardOptions } from './dashboard'

vi.mock('@/api/dashboard', () => ({
  fetchDashboard: vi.fn(),
}))

describe('dashboardOptions', () => {
  it('returns queryKey ["dashboard"]', () => {
    const opts = dashboardOptions()
    expect(opts.queryKey).toEqual(['dashboard'])
  })

  it('has staleTime of 5000', () => {
    const opts = dashboardOptions()
    expect(opts.staleTime).toBe(5_000)
  })

  it('has refetchOnWindowFocus set to "always"', () => {
    const opts = dashboardOptions()
    expect(opts.refetchOnWindowFocus).toBe('always')
  })

  it('has a queryFn', () => {
    const opts = dashboardOptions()
    expect(typeof opts.queryFn).toBe('function')
  })
})
