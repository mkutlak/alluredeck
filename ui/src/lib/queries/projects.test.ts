import { describe, it, expect, vi } from 'vitest'
import { projectIndexOptions, projectListOptions, projectParentsOptions } from './projects'
import { dashboardOptions } from './dashboard'
import { queryKeys } from '@/lib/query-keys'

vi.mock('@/api/projects', () => ({ getProjects: vi.fn(), getProjectIndex: vi.fn() }))
vi.mock('@/api/dashboard', () => ({ fetchDashboard: vi.fn() }))

// Fix 814b279: the project fetches used to share one ['projects'] key with
// different queryFns; critical lists got a 5s stale time and refetch on focus.
describe('project and dashboard query options', () => {
  it('give each fetch its own key under the prefix its invalidation matches', () => {
    const keys = [projectIndexOptions(), projectListOptions(), projectParentsOptions()].map(
      (o) => o.queryKey,
    )
    expect(new Set(keys.map((k) => JSON.stringify(k))).size).toBe(keys.length)
    for (const key of keys)
      expect(key.slice(0, queryKeys.projects.length)).toEqual(queryKeys.projects)
    // invalidateProjectQueries refreshes the dashboard through queryKeys.dashboard().
    expect(dashboardOptions().queryKey).toEqual(queryKeys.dashboard())
  })

  // Critical lists refetch on focus; the parent picker for dialogs does not.
  it.each([
    ['dashboardOptions', dashboardOptions, 'always'],
    ['projectListOptions', projectListOptions, 'always'],
    ['projectParentsOptions', projectParentsOptions, undefined],
  ] as const)('%s: 5s stale time, refetchOnWindowFocus %s', (_, options, refetchOnWindowFocus) => {
    expect(options()).toMatchObject({ queryFn: expect.any(Function), staleTime: 5_000 })
    expect(options().refetchOnWindowFocus).toBe(refetchOnWindowFocus)
  })
})
