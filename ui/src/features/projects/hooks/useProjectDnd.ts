import { useCallback, useMemo, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { setProjectParent, clearProjectParent } from '@/api/projects'
import { queryKeys } from '@/lib/query-keys'

export interface DndProject {
  slug: string
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
  projectSlug: string
  previousParentId: number | null
}

export function useProjectDnd(projects: DndProject[]) {
  const qc = useQueryClient()
  const undoRef = useRef<UndoState | null>(null)
  const lookup = useMemo(() => new Map(projects.map((p) => [p.slug, p])), [projects])

  const invalidate = useCallback(() => {
    void qc.invalidateQueries({ queryKey: queryKeys.projects })
    void qc.invalidateQueries({ queryKey: queryKeys.dashboard() })
  }, [qc])

  const setParent = useMutation({
    mutationFn: ({ slug, parentId }: { slug: string; parentId: number }) =>
      setProjectParent(slug, parentId),
    onSuccess: invalidate,
    onError: invalidate,
  })

  const clearParent = useMutation({
    mutationFn: (slug: string) => clearProjectParent(slug),
    onSuccess: invalidate,
    onError: invalidate,
  })

  const moveProject = useCallback(
    (
      draggedSlug: string,
      targetSlug: string | null,
    ): { success: boolean; targetLabel?: string } => {
      const dragged = lookup.get(draggedSlug)
      if (!dragged) return { success: false }

      if (targetSlug === null) {
        // Remove from group
        if (dragged.parentId == null) return { success: false }
        undoRef.current = { projectSlug: draggedSlug, previousParentId: dragged.parentId }
        clearParent.mutate(draggedSlug)
        return { success: true }
      }

      if (draggedSlug === targetSlug) return { success: false }

      const target = lookup.get(targetSlug)
      if (!target || !canBeDropTarget(target) || !canBeDragged(dragged)) {
        return { success: false }
      }

      undoRef.current = { projectSlug: draggedSlug, previousParentId: dragged.parentId }
      setParent.mutate({ slug: draggedSlug, parentId: target.projectId })
      return { success: true, targetLabel: target.slug }
    },
    [lookup, setParent, clearParent],
  )

  const undoLastMove = useCallback(() => {
    const last = undoRef.current
    if (!last) return
    undoRef.current = null
    if (last.previousParentId != null) {
      setParent.mutate({ slug: last.projectSlug, parentId: last.previousParentId })
    } else {
      clearParent.mutate(last.projectSlug)
    }
  }, [setParent, clearParent])

  return { moveProject, undoLastMove, lookup }
}
