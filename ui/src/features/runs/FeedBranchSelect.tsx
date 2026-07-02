import { useQueries, useQuery } from '@tanstack/react-query'

import { fetchBranches } from '@/api/branches'
import { queryKeys } from '@/lib/query-keys'
import { projectIndexOptions } from '@/lib/queries'
import { useUIStore } from '@/store/ui'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const ALL_BRANCHES_VALUE = '__all__'

export function FeedBranchSelect() {
  const runsFeedGroupIds = useUIStore((s) => s.runsFeedGroupIds)
  const selectedBranch = useUIStore((s) => s.selectedBranch)
  const setSelectedBranch = useUIStore((s) => s.setSelectedBranch)

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const allParentIds = (projectsResp?.data ?? [])
    .filter((p) => (p.children?.length ?? 0) > 0)
    .map((p) => p.project_id)

  const parentIds = runsFeedGroupIds.length > 0 ? runsFeedGroupIds : allParentIds

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

  const handleValueChange = (val: string) => {
    setSelectedBranch(val === ALL_BRANCHES_VALUE ? undefined : val)
  }

  const storedBranchInList = selectedBranch !== undefined && branchNames.includes(selectedBranch)
  const displayValue = storedBranchInList ? selectedBranch : ALL_BRANCHES_VALUE

  if (isLoading) {
    return (
      <div className="flex items-center gap-1.5">
        <span className="text-muted-foreground text-xs">Branch:</span>
        <Select disabled value={ALL_BRANCHES_VALUE}>
          <SelectTrigger className="h-8 min-w-28 w-auto text-xs" aria-label="Filter by branch">
            <SelectValue placeholder="All branches" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_BRANCHES_VALUE}>All branches</SelectItem>
          </SelectContent>
        </Select>
      </div>
    )
  }

  if (branchNames.length === 0) return null

  return (
    <div className="flex items-center gap-1.5">
      <span className="text-muted-foreground text-xs">Branch:</span>
      <Select value={displayValue} onValueChange={handleValueChange}>
        <SelectTrigger className="h-8 min-w-28 w-auto text-xs" aria-label="Filter by branch">
          <SelectValue placeholder="All branches" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_BRANCHES_VALUE}>All branches</SelectItem>
          {branchNames.map((name) => (
            <SelectItem key={name} value={name}>
              {name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
