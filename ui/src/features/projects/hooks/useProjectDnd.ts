import { useCallback, useMemo, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { setProjectParent, clearProjectParent } from '@/api/projects'
import { queryKeys } from '@/lib/query-keys'

export interface DndProject {
  /** Display name, as formatProjectLabel() gives it */
  label: string
  projectId: number
  parentId: number | null
  hasChildren: boolean
}

export function canBeDragged(p: DndProject): boolean {
  return !p.hasChildren
}

export function canBeDropTarget(p: DndProject): boolean {
  return p.parentId == null
}

interface UndoState {
  projectId: number
  previousParentId: number | null
}

export function useProjectDnd(projects: DndProject[]) {
  const qc = useQueryClient()
  const undoRef = useRef<UndoState | null>(null)
  const lookup = useMemo(() => new Map(projects.map((p) => [p.projectId, p])), [projects])

  const invalidate = useCallback(() => {
    void qc.invalidateQueries({ queryKey: queryKeys.projects })
    void qc.invalidateQueries({ queryKey: queryKeys.dashboard() })
  }, [qc])

  const setParent = useMutation({
    mutationFn: ({ projectId, parentId }: { projectId: number; parentId: number }) =>
      setProjectParent(projectId, parentId),
    onSuccess: invalidate,
    onError: invalidate,
  })

  const clearParent = useMutation({
    mutationFn: (projectId: number) => clearProjectParent(projectId),
    onSuccess: invalidate,
    onError: invalidate,
  })

  const moveProject = useCallback(
    (
      draggedId: number,
      targetId: number | null,
    ): { success: boolean; targetLabel?: string } => {
      const dragged = lookup.get(draggedId)
      if (!dragged) return { success: false }

      if (targetId === null) {
        // Remove from group
        if (dragged.parentId == null) return { success: false }
        undoRef.current = { projectId: draggedId, previousParentId: dragged.parentId }
        clearParent.mutate(draggedId)
        return { success: true }
      }

      if (draggedId === targetId) return { success: false }

      const target = lookup.get(targetId)
      if (!target || !canBeDropTarget(target) || !canBeDragged(dragged)) {
        return { success: false }
      }

      undoRef.current = { projectId: draggedId, previousParentId: dragged.parentId }
      setParent.mutate({ projectId: draggedId, parentId: target.projectId })
      return { success: true, targetLabel: target.label }
    },
    [lookup, setParent, clearParent],
  )

  const undoLastMove = useCallback(() => {
    const last = undoRef.current
    if (!last) return
    undoRef.current = null
    if (last.previousParentId != null) {
      setParent.mutate({ projectId: last.projectId, parentId: last.previousParentId })
    } else {
      clearParent.mutate(last.projectId)
    }
  }, [setParent, clearParent])

  return { moveProject, undoLastMove, lookup }
}
