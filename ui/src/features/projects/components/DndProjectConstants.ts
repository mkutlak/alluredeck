import { createContext, useContext } from 'react'
import type { DndProject } from '../hooks/useProjectDnd'

export interface ProjectDndContextValue {
  isDragging: boolean
  activeSlug: string | null
  overSlug: string | null
  getProject: (slug: string) => DndProject | undefined
  isProjectDraggable: (slug: string) => boolean
  isProjectDropTarget: (slug: string) => boolean
}

export const ProjectDndContext = createContext<ProjectDndContextValue>({
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
