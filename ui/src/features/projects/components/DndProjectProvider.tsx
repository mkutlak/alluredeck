import { useState, useCallback } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  pointerWithin,
  type DragStartEvent,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  useProjectDnd,
  canBeDragged,
  canBeDropTarget,
  type DndProject,
} from '../hooks/useProjectDnd'
import { ProjectDndContext } from './DndProjectConstants'
import { DragOverlayCard } from './DragOverlayCard'
import { toast } from '@/components/ui/use-toast'
import { ToastAction } from '@/components/ui/toast'


interface DndProjectProviderProps {
  projects: DndProject[]
  children: React.ReactNode
}

export function DndProjectProvider({ projects, children }: DndProjectProviderProps) {
  const [activeId, setActiveId] = useState<number | null>(null)
  const { moveProject, undoLastMove, lookup } = useProjectDnd(projects)

  const pointerSensor = useSensor(PointerSensor, {
    activationConstraint: { distance: 12 },
  })
  const keyboardSensor = useSensor(KeyboardSensor)
  const sensors = useSensors(pointerSensor, keyboardSensor)

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveId(Number(event.active.id))
  }, [])

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const draggedId = Number(event.active.id)
      const draggedSlug = lookup.get(draggedId)?.slug
      const overId = event.over?.id

      setActiveId(null)

      if (overId === undefined) return

      if (overId === 'no-group') {
        const result = moveProject(draggedId, null)
        if (result.success) {
          toast({
            title: `Removed ${draggedSlug} from group`,
            action: (
              <ToastAction altText="Undo" onClick={() => undoLastMove()}>
                Undo
              </ToastAction>
            ),
          })
        }
        return
      }

      if (draggedId === Number(overId)) return

      const result = moveProject(draggedId, Number(overId))
      if (result.success) {
        toast({
          title: `Moved ${draggedSlug} → ${result.targetLabel}`,
          action: (
            <ToastAction altText="Undo" onClick={() => undoLastMove()}>
              Undo
            </ToastAction>
          ),
        })
      }
    },
    [lookup, moveProject, undoLastMove],
  )

  const handleDragCancel = useCallback(() => {
    setActiveId(null)
  }, [])

  const activeProject = activeId != null ? lookup.get(activeId) : undefined

  const getProject = useCallback((projectId: number) => lookup.get(projectId), [lookup])

  const isProjectDraggable = useCallback(
    (projectId: number) => {
      const p = lookup.get(projectId)
      return p ? canBeDragged(p) : false
    },
    [lookup],
  )

  const isProjectDropTarget = useCallback(
    (projectId: number) => {
      const p = lookup.get(projectId)
      return p ? canBeDropTarget(p) : false
    },
    [lookup],
  )

  return (
    <ProjectDndContext.Provider
      value={{
        isDragging: activeId !== null,
        activeId,
        getProject,
        isProjectDraggable,
        isProjectDropTarget,
      }}
    >
      <DndContext
        sensors={sensors}
        collisionDetection={pointerWithin}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
      >
        {children}
        <DragOverlay>
          {activeProject ? <DragOverlayCard slug={activeProject.slug} /> : null}
        </DragOverlay>
      </DndContext>
    </ProjectDndContext.Provider>
  )
}
