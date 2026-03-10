import type { ReactElement } from 'react'
import { useMemo, useEffect, useRef, useState } from 'react'
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

  // Derive the default branch name once branches are loaded
  const defaultBranchName = useMemo(
    () => branches?.find((b) => b.is_default)?.name,
    [branches],
  )

  // Track when the user explicitly selects "All branches" so we can show it correctly
  // even when selectedBranch is undefined (which also describes the initial unset state).
  const [userChoseAll, setUserChoseAll] = useState(false)

  // Notify parent once when branches load and there is a default to auto-select
  const notifiedRef = useRef(false)
  useEffect(() => {
    if (notifiedRef.current) return
    if (defaultBranchName === undefined) return
    if (selectedBranch !== undefined) return
    notifiedRef.current = true
    onBranchChange(defaultBranchName)
  }, [defaultBranchName, selectedBranch, onBranchChange])

  const handleValueChange = (val: string) => {
    setUserChoseAll(val === ALL_BRANCHES_VALUE)
    onBranchChange(val === ALL_BRANCHES_VALUE ? undefined : val)
  }

  // If the user explicitly chose "All branches" show that; otherwise honour the
  // controlled prop and fall back to the default branch (pre-selection) then "all".
  const displayValue = userChoseAll
    ? ALL_BRANCHES_VALUE
    : selectedBranch !== undefined
      ? selectedBranch
      : (defaultBranchName ?? ALL_BRANCHES_VALUE)

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
