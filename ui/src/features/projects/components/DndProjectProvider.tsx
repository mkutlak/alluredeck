import { createContext, useContext, useState, useCallback } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  pointerWithin,
  type DragStartEvent,
  type DragOverEvent,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  useProjectDnd,
  canBeDragged,
  canBeDropTarget,
  type DndProject,
} from '../hooks/useProjectDnd'
import { DragOverlayCard } from './DragOverlayCard'
import { toast } from '@/components/ui/use-toast'
import { ToastAction } from '@/components/ui/toast'

interface ProjectDndContextValue {
  isDragging: boolean
  activeSlug: string | null
  overSlug: string | null
  getProject: (slug: string) => DndProject | undefined
  isProjectDraggable: (slug: string) => boolean
  isProjectDropTarget: (slug: string) => boolean
}

const ProjectDndContext = createContext<ProjectDndContextValue>({
  isDragging: false,
  activeSlug: null,
  overSlug: null,
  getProject: () => undefined,
  isProjectDraggable: () => false,
  isProjectDropTarget: () => false,
})

export function useProjectDndContext() {
  return useContext(ProjectDndContext)
}

interface DndProjectProviderProps {
  projects: DndProject[]
  children: React.ReactNode
}

export function DndProjectProvider({ projects, children }: DndProjectProviderProps) {
  const [activeSlug, setActiveSlug] = useState<string | null>(null)
  const [overSlug, setOverSlug] = useState<string | null>(null)
  const { moveProject, undoLastMove, lookup } = useProjectDnd(projects)

  const pointerSensor = useSensor(PointerSensor, {
    activationConstraint: { distance: 12 },
  })
  const keyboardSensor = useSensor(KeyboardSensor)
  const sensors = useSensors(pointerSensor, keyboardSensor)

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveSlug(String(event.active.id))
  }, [])

  const handleDragOver = useCallback((event: DragOverEvent) => {
    setOverSlug(event.over ? String(event.over.id) : null)
  }, [])

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const draggedSlug = String(event.active.id)
      const targetId = event.over ? String(event.over.id) : null

      setActiveSlug(null)
      setOverSlug(null)

      if (!targetId) return

      if (targetId === 'no-group') {
        const result = moveProject(draggedSlug, null)
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

      if (draggedSlug === targetId) return

      const result = moveProject(draggedSlug, targetId)
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
    [moveProject, undoLastMove],
  )

  const handleDragCancel = useCallback(() => {
    setActiveSlug(null)
    setOverSlug(null)
  }, [])

  const activeProject = activeSlug ? lookup.get(activeSlug) : undefined

  const getProject = useCallback((slug: string) => lookup.get(slug), [lookup])

  const isProjectDraggable = useCallback(
    (slug: string) => {
      const p = lookup.get(slug)
      return p ? canBeDragged(p) : false
    },
    [lookup],
  )

  const isProjectDropTarget = useCallback(
    (slug: string) => {
      const p = lookup.get(slug)
      return p ? canBeDropTarget(p) : false
    },
    [lookup],
  )

  return (
    <ProjectDndContext.Provider
      value={{
        isDragging: activeSlug !== null,
        activeSlug,
        overSlug,
        getProject,
        isProjectDraggable,
        isProjectDropTarget,
      }}
    >
      <DndContext
        sensors={sensors}
        collisionDetection={pointerWithin}
        onDragStart={handleDragStart}
        onDragOver={handleDragOver}
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
