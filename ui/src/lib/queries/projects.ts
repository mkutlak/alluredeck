import { queryOptions } from '@tanstack/react-query'
import { getProjectIndex, getProjects } from '@/api/projects'

/** All projects (unpaginated) — sidebar, switcher, admin cards, lookups */
export function projectIndexOptions() {
  return queryOptions({
    queryKey: ['projects', 'index'] as const,
    queryFn: () => getProjectIndex(),
    staleTime: 10_000,
    refetchOnWindowFocus: 'always' as const,
  })
}

/** Paginated projects — projects page list/grid */
export function projectListOptions() {
  return queryOptions({
    queryKey: ['projects'] as const,
    queryFn: () => getProjects(),
    staleTime: 5_000,
    refetchOnWindowFocus: 'always' as const,
  })
}

/** Parent-eligible projects for dialogs */
export function projectParentsOptions() {
  return queryOptions({
    queryKey: ['projects', 'parents'] as const,
    queryFn: () => getProjects(1, 200),
    staleTime: 5_000,
  })
}
