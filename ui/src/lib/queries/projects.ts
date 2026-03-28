import { queryOptions } from '@tanstack/react-query'
import { getProjects } from '@/api/projects'

/** All projects — sidebar, switcher, projects page, overview tab */
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
