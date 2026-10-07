import { useQuery } from '@tanstack/react-query'

import { projectIndexOptions } from '@/lib/queries'
import { useFeedBranches } from './useFeedBranches'
import { useFeedParams } from './useFeedParams'
import { Combobox } from '@/components/ui/combobox'

export function FeedBranchSelect() {
  const { branch, setBranch, groupIds } = useFeedParams()

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const allParentIds = (projectsResp?.data ?? [])
    .filter((p) => (p.children?.length ?? 0) > 0)
    .map((p) => p.project_id)

  const parentIds = groupIds.length > 0 ? groupIds : allParentIds

  const { branchNames, isLoading } = useFeedBranches(parentIds)

  // Kept while a branch is set, or a URL branch no group has could not be cleared.
  if (!isLoading && branchNames.length === 0 && !branch) return null

  return (
    <div className="flex items-center gap-1.5">
      <span className="text-fact text-xs">Branch:</span>
      <Combobox
        aria-label="Filter by branch"
        options={branchNames.map((name) => ({ value: name, label: name }))}
        value={branch ?? null}
        onChange={(next) => setBranch(next ?? undefined)}
        placeholder="All branches"
        searchPlaceholder="Search branches…"
        emptyText="No matching branches."
        allowClear
        disabled={isLoading}
        // "All branches" is a real state, not a hint: same ink as the group filter.
        className="[&>span]:text-foreground h-8 w-auto max-w-72 min-w-56 px-3 text-xs"
      />
    </div>
  )
}
