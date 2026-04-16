import { useDroppable } from '@dnd-kit/core'
import { FolderX } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useProjectDndContext } from './DndProjectConstants'

export function NoGroupDropZone() {
  const { isDragging, activeSlug, getProject } = useProjectDndContext()
  const { setNodeRef, isOver } = useDroppable({ id: 'no-group' })

  const activeProject = activeSlug ? getProject(activeSlug) : undefined
  const shouldShow = isDragging && activeProject?.parentId != null

  if (!shouldShow) return null

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-3 text-sm transition-colors',
        isOver
          ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-950/30 dark:text-blue-400'
          : 'border-muted-foreground/30 text-muted-foreground',
      )}
    >
      <FolderX size={16} />
      Drop here to remove from group
    </div>
  )
}
