import { createContext, useContext } from 'react'
import type { DndProject } from '../hooks/useProjectDnd'

export interface ProjectDndContextValue {
  isDragging: boolean
  activeId: number | null
  getProject: (projectId: number) => DndProject | undefined
  isProjectDraggable: (projectId: number) => boolean
  isProjectDropTarget: (projectId: number) => boolean
}

export const ProjectDndContext = createContext<ProjectDndContextValue>({
  isDragging: false,
  activeId: null,
  getProject: () => undefined,
  isProjectDraggable: () => false,
  isProjectDropTarget: () => false,
})

export function useProjectDndContext() {
  return useContext(ProjectDndContext)
}
