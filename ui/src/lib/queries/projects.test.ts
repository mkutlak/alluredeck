import { describe, it, expect, vi } from 'vitest'
import { projectListOptions, projectParentsOptions } from './projects'

vi.mock('@/api/projects', () => ({
  getProjects: vi.fn(),
}))

describe('projectListOptions', () => {
  it('returns queryKey ["projects"]', () => {
    const opts = projectListOptions()
    expect(opts.queryKey).toEqual(['projects'])
  })

  it('has staleTime of 5000', () => {
    const opts = projectListOptions()
    expect(opts.staleTime).toBe(5_000)
  })

  it('has refetchOnWindowFocus set to "always"', () => {
    const opts = projectListOptions()
    expect(opts.refetchOnWindowFocus).toBe('always')
  })

  it('has a queryFn', () => {
    const opts = projectListOptions()
    expect(typeof opts.queryFn).toBe('function')
  })
})

describe('projectParentsOptions', () => {
  it('returns queryKey ["projects", "parents"]', () => {
    const opts = projectParentsOptions()
    expect(opts.queryKey).toEqual(['projects', 'parents'])
  })

  it('has staleTime of 5000', () => {
    const opts = projectParentsOptions()
    expect(opts.staleTime).toBe(5_000)
  })

  it('has a queryFn', () => {
    const opts = projectParentsOptions()
    expect(typeof opts.queryFn).toBe('function')
  })

  it('does not set refetchOnWindowFocus to "always"', () => {
    const opts = projectParentsOptions()
    expect(opts.refetchOnWindowFocus).toBeUndefined()
  })
})
