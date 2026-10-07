import { useQuery } from '@tanstack/react-query'

import { projectIndexOptions } from '@/lib/queries'
import { useUIStore } from '@/store/ui'
import { useFeedBranches } from './useFeedBranches'
import { Combobox } from '@/components/ui/combobox'

export function FeedBranchSelect() {
  const runsFeedGroupIds = useUIStore((s) => s.runsFeedGroupIds)
  const selectedBranch = useUIStore((s) => s.selectedBranch)
  const setSelectedBranch = useUIStore((s) => s.setSelectedBranch)

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const allParentIds = (projectsResp?.data ?? [])
    .filter((p) => (p.children?.length ?? 0) > 0)
    .map((p) => p.project_id)

  const parentIds = runsFeedGroupIds.length > 0 ? runsFeedGroupIds : allParentIds

  const { branchNames, isLoading } = useFeedBranches(parentIds)

  if (!isLoading && branchNames.length === 0) return null

  // The stored branch is shared across pages; one this feed lacks reads as "All branches".
  const storedBranchInList = selectedBranch !== undefined && branchNames.includes(selectedBranch)

  return (
    <div className="flex items-center gap-1.5">
      <span className="text-fact text-xs">Branch:</span>
      <Combobox
        aria-label="Filter by branch"
        options={branchNames.map((name) => ({ value: name, label: name }))}
        value={storedBranchInList ? selectedBranch : null}
        onChange={(branch) => setSelectedBranch(branch ?? undefined)}
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
