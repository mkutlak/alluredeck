import { useState } from 'react'
import { NavLink } from 'react-router'
import { ChevronDown, ChevronRight, FileText, Folder, FolderInput, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'
import { useDraggable, useDroppable } from '@dnd-kit/core'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TableCell, TableRow } from '@/components/ui/table'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { RenameProjectDialog } from '@/features/projects/RenameProjectDialog'
import { SetParentDialog } from '@/features/projects/SetParentDialog'
import { DeleteProjectDialog } from '@/features/projects/DeleteProjectDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import { useProjectDndContext } from '@/features/projects/components/DndProjectConstants'
import { getPassRateBadgeClass } from '@/lib/status-colors'
import { cn, formatPassRate } from '@/lib/utils'
import type { DashboardProjectEntry } from '@/types/api'
import { formatProjectLabel } from '@/lib/projectLabel'
import { getPassRate, getProjectType } from './sort'

interface DashboardProjectRowProps {
  project: DashboardProjectEntry
  isAdmin: boolean
  onDrillDown?: () => void
  allProjects?: readonly DashboardProjectEntry[]
  /** Grouped-mode only: render a chevron expand/collapse button */
  showChevron?: boolean
  /** Whether this group row is currently expanded */
  isExpanded?: boolean
  /** Called when the chevron is clicked */
  onToggleExpand?: () => void
  /** Render this row as an indented child row */
  isChild?: boolean
}

export function DashboardProjectRow({
  project,
  isAdmin,
  onDrillDown,
  allProjects,
  showChevron,
  isExpanded,
  onToggleExpand,
  isChild,
}: DashboardProjectRowProps) {
  const [cleanMode, setCleanMode] = useState<'results' | 'history' | null>(null)
  const [renameOpen, setRenameOpen] = useState(false)
  const [moveOpen, setMoveOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const rate = getPassRate(project)
  const type = getProjectType(project)

  const { isProjectDraggable, isProjectDropTarget } = useProjectDndContext()

  const draggable = isProjectDraggable(project.slug)
  const dropTarget = isProjectDropTarget(project.slug)

  const {
    attributes: { role: _role, tabIndex: _tabIndex, ...dragAttributes },
    listeners,
    setNodeRef: setDragRef,
    isDragging,
  } = useDraggable({
    id: project.slug,
    disabled: !draggable,
  })

  const { setNodeRef: setDropRef, isOver } = useDroppable({
    id: project.slug,
    disabled: !dropTarget,
  })

  const setNodeRef = (el: HTMLTableRowElement | null) => {
    setDragRef(el)
    setDropRef(el)
  }

  return (
    <>
      <TableRow
        ref={setNodeRef}
        {...(draggable ? { ...listeners, ...dragAttributes } : {})}
        className={cn(
          onDrillDown ? 'cursor-pointer' : '',
          isDragging && 'opacity-40',
          isOver && dropTarget && 'scale-[1.02] ring-2 ring-blue-500',
          draggable && !onDrillDown && 'cursor-grab',
          isChild && 'text-muted-foreground',
        )}
        onClick={onDrillDown}
      >
        <TableCell className={cn('font-medium', isChild && 'pl-8')}>
          <div className="flex items-center gap-2">
            {showChevron && (
              <button
                type="button"
                aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${project.slug}`}
                className="text-muted-foreground hover:text-foreground shrink-0 rounded p-0.5 transition-colors"
                onClick={(e) => {
                  e.stopPropagation()
                  onToggleExpand?.()
                }}
              >
                {isExpanded ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </button>
            )}
            {project.is_group ? (
              <Folder data-testid="icon-folder" className="text-muted-foreground h-4 w-4 shrink-0" />
            ) : (
              <FileText data-testid="icon-file-text" className="text-muted-foreground h-4 w-4 shrink-0" />
            )}
            {project.is_group ? (
              <span>{project.slug}</span>
            ) : (
              <div className="flex flex-col">
                <NavLink
                  to={`/projects/${project.project_id}`}
                  className="hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {formatProjectLabel(project, allProjects)}
                </NavLink>
                {project.display_name && project.display_name !== project.slug && (
                  <span className="text-muted-foreground text-xs">{project.slug.split('--')[0]}</span>
                )}
              </div>
            )}
          </div>
        </TableCell>
        <TableCell>
          <span className="text-muted-foreground text-sm">{type}</span>
        </TableCell>
        <TableCell>
          {rate != null ? (
            <Badge
              variant={rate >= 90 ? 'default' : rate >= 70 ? 'secondary' : 'destructive'}
              className={getPassRateBadgeClass(rate)}
            >
              {formatPassRate(rate)}
            </Badge>
          ) : (
            <span className="text-muted-foreground text-sm">&mdash;</span>
          )}
        </TableCell>
        {isAdmin && (
          <TableCell onClick={(e) => e.stopPropagation()}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  aria-label="Project actions"
                >
                  <MoreHorizontal size={14} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {project.is_group ? (
                  <>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setCleanMode('results')}
                    >
                      <Trash2 size={14} className="mr-2" />
                      Clean all results
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setCleanMode('history')}
                    >
                      <Trash2 size={14} className="mr-2" />
                      Clean all history
                    </DropdownMenuItem>
                  </>
                ) : (
                  <>
                    <DropdownMenuItem onClick={() => setRenameOpen(true)}>
                      <Pencil size={14} className="mr-2" />
                      Rename project
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setMoveOpen(true)}>
                      <FolderInput size={14} className="mr-2" />
                      Move to group...
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setDeleteOpen(true)}
                    >
                      <Trash2 size={14} className="mr-2" />
                      Delete project
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </TableCell>
        )}
      </TableRow>
      {cleanMode && (
        <CleanDialog
          projectId={project.slug}
          mode={cleanMode}
          open={!!cleanMode}
          onOpenChange={(o) => {
            if (!o) setCleanMode(null)
          }}
          groupMode
        />
      )}
      {renameOpen && (
        <RenameProjectDialog
          projectId={project.slug}
          numericId={project.project_id}
          open={renameOpen}
          onOpenChange={setRenameOpen}
        />
      )}
      {moveOpen && (
        <SetParentDialog
          projectId={project.slug}
          open={moveOpen}
          onOpenChange={setMoveOpen}
        />
      )}
      {deleteOpen && (
        <DeleteProjectDialog
          projectId={project.slug}
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
        />
      )}
    </>
  )
}
