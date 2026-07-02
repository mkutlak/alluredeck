import { useParams } from 'react-router'
import { BranchSelector } from '@/features/projects/BranchSelector'
import { useUIStore } from '@/store/ui'

export function BranchSelect() {
  const { id: projectId } = useParams<{ id: string }>()
  const selectedBranch = useUIStore((s) => s.selectedBranch)
  const setSelectedBranch = useUIStore((s) => s.setSelectedBranch)

  if (!projectId) return null

  return (
    <BranchSelector
      projectId={projectId}
      selectedBranch={selectedBranch}
      onBranchChange={setSelectedBranch}
    />
  )
}
