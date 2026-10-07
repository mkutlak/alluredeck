import { useQueries } from '@tanstack/react-query'

import { fetchBranches } from '@/api/branches'
import { queryKeys } from '@/lib/query-keys'

export interface FeedBranches {
  branchNames: string[]
  isLoading: boolean
}

/**
 * Fetches and unions branch names across the given parent project ids, for the
 * options of FeedBranchSelect. The feed itself filters by the URL's branch.
 */
export function useFeedBranches(parentIds: number[]): FeedBranches {
  const branchQueries = useQueries({
    queries: parentIds.map((id) => ({
      queryKey: queryKeys.branches.list(String(id)),
      queryFn: () => fetchBranches(String(id)),
      staleTime: 60_000,
    })),
  })

  const isLoading = branchQueries.some((q) => q.isLoading)
  const branchNames = Array.from(
    new Set(branchQueries.flatMap((q) => q.data?.map((b) => b.name) ?? [])),
  ).sort()

  return { branchNames, isLoading }
}
