import { useState } from 'react'
import { Link } from 'react-router'
import { FolderInput, FolderOpen, Pencil, Trash2, MoreHorizontal } from 'lucide-react'
import { useDraggable, useDroppable } from '@dnd-kit/core'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DeleteProjectDialog } from './DeleteProjectDialog'
import { RenameProjectDialog } from './RenameProjectDialog'
import { SetParentDialog } from './SetParentDialog'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { useProjectDndContext } from './components/DndProjectConstants'
import { cn } from '@/lib/utils'

interface ProjectCardProps {
  projectId: string
  numericId: number
  storageKey?: string
}

export function ProjectCard({ projectId, numericId, storageKey }: ProjectCardProps) {
  const isAdmin = useAuthStore(selectIsAdmin)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [renameOpen, setRenameOpen] = useState(false)
  const [moveOpen, setMoveOpen] = useState(false)

  const { activeSlug, overSlug, isProjectDraggable, isProjectDropTarget } = useProjectDndContext()

  const draggable = isProjectDraggable(projectId)
  const droppable = isProjectDropTarget(projectId)

  const { setNodeRef: setDragRef, listeners, attributes, isDragging } = useDraggable({
    id: projectId,
    disabled: !draggable,
  })

  const { setNodeRef: setDropRef, isOver } = useDroppable({
    id: projectId,
    disabled: !droppable,
  })

  const isActiveDropTarget = isOver && overSlug === projectId && activeSlug !== projectId

  const combinedRef = (node: HTMLDivElement | null) => {
    setDragRef(node)
    setDropRef(node)
  }

  return (
    <>
      <Card
        ref={combinedRef}
        className={cn(
          'group relative transition-shadow hover:shadow-md',
          isDragging && 'opacity-40',
          isActiveDropTarget && 'scale-[1.02] ring-2 ring-blue-500',
          draggable && 'cursor-grab',
        )}
        data-testid="project-card"
        data-project-slug={projectId}
        {...(draggable ? { ...listeners, ...attributes } : {})}
      >
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between gap-2">
            <div className="flex min-w-0 items-center gap-2">
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <FolderOpen size={16} className="text-muted-foreground shrink-0 cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="bottom" className="font-mono text-xs">
                    <p>ID: {storageKey ?? projectId}</p>
                    <p>Storage: projects/{storageKey ?? projectId}/</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
              <CardTitle className="truncate text-sm font-medium">{projectId}</CardTitle>
            </div>
            {isAdmin && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
                    aria-label="Project actions"
                  >
                    <MoreHorizontal size={14} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => setRenameOpen(true)}>
                    <Pencil size={14} />
                    Rename project
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setMoveOpen(true)}>
                    <FolderInput size={14} />
                    Move to group...
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onClick={() => setDeleteOpen(true)}
                  >
                    <Trash2 size={14} />
                    Delete project
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <Badge variant="secondary" className="font-mono text-xs">
            {projectId}
          </Badge>
          <div className="mt-4">
            <Button asChild size="sm" variant="outline" className="w-full">
              <Link to={`/projects/${numericId}`}>View reports</Link>
            </Button>
          </div>
          {isActiveDropTarget && (
            <p className="mt-2 text-center text-xs text-blue-500">Drop to move into {projectId}</p>
          )}
        </CardContent>
      </Card>

      {isAdmin && (
        <>
          <RenameProjectDialog
            projectId={projectId}
            numericId={numericId}
            open={renameOpen}
            onOpenChange={setRenameOpen}
          />
          <DeleteProjectDialog
            projectId={projectId}
            open={deleteOpen}
            onOpenChange={setDeleteOpen}
          />
          <SetParentDialog projectId={projectId} open={moveOpen} onOpenChange={setMoveOpen} />
        </>
      )}
    </>
  )
}
