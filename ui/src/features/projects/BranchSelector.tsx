import type { ReactElement } from 'react'
import { useQuery } from '@tanstack/react-query'
import { queryKeys } from '@/lib/query-keys'
import { fetchBranches } from '@/api/branches'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const ALL_BRANCHES_VALUE = '__all__'

export function BranchSelector({
  projectId,
  selectedBranch,
  onBranchChange,
}: {
  projectId: string
  selectedBranch: string | undefined
  onBranchChange: (branch: string | undefined) => void
}): ReactElement | null {
  const { data: branches, isLoading } = useQuery({
    queryKey: queryKeys.branches.list(projectId),
    queryFn: () => fetchBranches(projectId),
    enabled: !!projectId,
    staleTime: 60_000,
  })

  const handleValueChange = (val: string) => {
    onBranchChange(val === ALL_BRANCHES_VALUE ? undefined : val)
  }

  const storedBranchInList =
    selectedBranch !== undefined && branches?.some((b) => b.name === selectedBranch)
  const displayValue = storedBranchInList ? selectedBranch : ALL_BRANCHES_VALUE

  // While loading, show a disabled placeholder trigger so tests can find the combobox
  if (isLoading) {
    return (
      <Select disabled value={ALL_BRANCHES_VALUE}>
        <SelectTrigger className="w-48" aria-label="Filter by branch">
          <SelectValue placeholder="All branches" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_BRANCHES_VALUE}>All branches</SelectItem>
        </SelectContent>
      </Select>
    )
  }

  // No branches available — render nothing
  if (!branches || branches.length === 0) return null

  return (
    <Select value={displayValue} onValueChange={handleValueChange}>
      <SelectTrigger className="w-48" aria-label="Filter by branch">
        <SelectValue placeholder="All branches" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL_BRANCHES_VALUE}>All branches</SelectItem>
        {branches.map((b) => (
          <SelectItem key={b.name} value={b.name}>
            {b.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
