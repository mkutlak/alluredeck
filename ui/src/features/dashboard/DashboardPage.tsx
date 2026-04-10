import { useState, useMemo } from 'react'
import { NavLink, useSearchParams } from 'react-router'
import { ArrowUpDown, ChevronRight, Folder, FolderInput, MoreHorizontal, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useDraggable, useDroppable } from '@dnd-kit/core'
import { dashboardOptions } from '@/lib/queries'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { CreateProjectDialog } from '@/features/projects/CreateProjectDialog'
import { DeleteProjectDialog } from '@/features/projects/DeleteProjectDialog'
import { RenameProjectDialog } from '@/features/projects/RenameProjectDialog'
import { SetParentDialog } from '@/features/projects/SetParentDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'
import { getPassRateBadgeClass } from '@/lib/status-colors'
import { DndProjectProvider, useProjectDndContext } from '@/features/projects/components/DndProjectProvider'
import { NoGroupDropZone } from '@/features/projects/components/NoGroupDropZone'
import type { DndProject } from '@/features/projects/hooks/useProjectDnd'
import { cn } from '@/lib/utils'
import type { DashboardProjectEntry } from '@/types/api'

/** Returns the human-friendly label for a project: display_name if available, otherwise slug. */
function projectLabel(p: { slug: string; display_name?: string; project_id: number }) {
  return p.display_name || p.slug || String(p.project_id)
}

type SortField = 'name' | 'type' | 'pass_rate'
type SortDir = 'asc' | 'desc'
type ViewMode = 'grouped' | 'all'

function getProjectType(p: DashboardProjectEntry): string {
  if (p.is_group) return 'Group'
  if (p.report_type === 'playwright') return 'Playwright'
  return 'Allure'
}

function getPassRate(p: DashboardProjectEntry): number | null {
  if (p.is_group) return p.aggregate?.pass_rate ?? null
  return p.latest_build?.pass_rate ?? null
}

function compareRows(a: DashboardProjectEntry, b: DashboardProjectEntry, field: SortField, dir: SortDir): number {
  const cmp =
    field === 'name'
      ? a.slug.localeCompare(b.slug)
      : field === 'type'
        ? getProjectType(a).localeCompare(getProjectType(b))
        : (getPassRate(a) ?? -1) - (getPassRate(b) ?? -1)
  return dir === 'asc' ? cmp : -cmp
}

export function DashboardPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [viewMode, setViewMode] = useState<ViewMode>('grouped')
  const [sortField, setSortField] = useState<SortField>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [searchParams, setSearchParams] = useSearchParams()
  const groupIdRaw = searchParams.get('group')
  const groupId = groupIdRaw != null ? parseInt(groupIdRaw, 10) : null
  const isAdmin = useAuthStore(selectIsAdmin)

  const { data, isLoading, isFetching, isError, refetch } = useQuery(dashboardOptions())

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortField(field)
      setSortDir('asc')
    }
  }

  const rows = useMemo(() => {
    if (!data) return []
    const { projects } = data

    // Drill-down: show only children of the selected group
    if (groupId != null && !isNaN(groupId)) {
      const group = projects.find((p) => p.project_id === groupId && p.is_group)
      const children = group?.children ?? []
      const filtered = search
        ? children.filter((c) => {
            const q = search.toLowerCase()
            return c.slug.toLowerCase().includes(q) || (c.display_name ?? '').toLowerCase().includes(q)
          })
        : children
      return [...filtered].sort((a, b) => compareRows(a, b, sortField, sortDir))
    }

    // All mode: flatten everything (deduplicate by project_id)
    if (viewMode === 'all') {
      const flat: DashboardProjectEntry[] = []
      const seen = new Set<number>()
      for (const p of projects) {
        if (p.is_group && p.children) {
          for (const c of p.children) {
            if (!seen.has(c.project_id)) {
              seen.add(c.project_id)
              flat.push(c)
            }
          }
        } else if (!p.is_group && !seen.has(p.project_id)) {
          seen.add(p.project_id)
          flat.push(p)
        }
      }
      const filtered = search
        ? flat.filter((p) => {
            const q = search.toLowerCase()
            return p.slug.toLowerCase().includes(q) || (p.display_name ?? '').toLowerCase().includes(q)
          })
        : flat
      return [...filtered].sort((a, b) => compareRows(a, b, sortField, sortDir))
    }

    // Grouped mode: groups first, then ungrouped projects
    const filtered = search
      ? projects.filter((p) => {
          const q = search.toLowerCase()
          const nameMatch = p.slug.toLowerCase().includes(q) || (p.display_name ?? '').toLowerCase().includes(q)
          if (nameMatch) return true
          if (p.is_group && p.children) {
            return p.children.some((c) => {
              const q = search.toLowerCase()
              return c.slug.toLowerCase().includes(q) || (c.display_name ?? '').toLowerCase().includes(q)
            })
          }
          return false
        })
      : projects

    const groups = filtered.filter((p) => p.is_group)
    const standalone = filtered.filter((p) => !p.is_group)
    return [
      ...groups.sort((a, b) => compareRows(a, b, sortField, sortDir)),
      ...standalone.sort((a, b) => compareRows(a, b, sortField, sortDir)),
    ]
  }, [data, groupId, viewMode, search, sortField, sortDir])

  const dndProjects: DndProject[] = useMemo(() => {
    if (!data) return []
    const items: DndProject[] = []
    for (const p of data.projects) {
      items.push({
        slug: p.slug,
        projectId: p.project_id,
        parentId: null, // top-level entries in dashboard
        hasChildren: !!(p.is_group && p.children?.length),
      })
      if (p.children) {
        for (const child of p.children) {
          items.push({
            slug: child.slug,
            projectId: child.project_id,
            parentId: p.project_id,
            hasChildren: false,
          })
        }
      }
    }
    return items
  }, [data])

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="mb-6">
          <h1 className="text-2xl font-bold">Projects</h1>
        </div>
        <div className="space-y-2">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-10 animate-pulse rounded-md" />
          ))}
        </div>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">Failed to load dashboard.</p>
        <Button onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  if (!data || data.projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">No projects yet</p>
        <p className="text-muted-foreground text-sm">
          {isAdmin ? 'Create a project to get started.' : 'Ask an admin to create a project.'}
        </p>
        {isAdmin && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus />
            Create first project
          </Button>
        )}
        <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
      </div>
    )
  }

  return (
    <div className="p-6">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          {groupId != null && !isNaN(groupId) ? (
            <>
              <button
                onClick={() => setSearchParams({})}
                className="text-muted-foreground text-2xl font-bold hover:underline"
              >
                Projects
              </button>
              <ChevronRight className="text-muted-foreground h-5 w-5" />
              <h1 className="text-2xl font-bold">
                {data?.projects.find((p) => p.project_id === groupId)?.slug ?? String(groupId)}
              </h1>
            </>
          ) : (
            <h1 className="text-2xl font-bold">Projects</h1>
          )}
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="text-muted-foreground absolute left-2.5 top-2.5 h-4 w-4" />
            <Input
              placeholder="Search..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-48 pl-8"
            />
          </div>
          {(groupId == null || isNaN(groupId)) && (
            <div className="flex rounded-md border">
              <Button
                size="sm"
                variant="ghost"
                className={`rounded-r-none border-r px-3 ${viewMode === 'grouped' ? 'bg-muted font-semibold' : ''}`}
                onClick={() => setViewMode('grouped')}
              >
                Grouped
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className={`rounded-l-none px-3 ${viewMode === 'all' ? 'bg-muted font-semibold' : ''}`}
                onClick={() => setViewMode('all')}
              >
                All
              </Button>
            </div>
          )}
          <Button variant="outline" size="icon" onClick={() => refetch()} aria-label="Refresh">
            <RefreshCw className={isFetching ? 'animate-spin' : ''} />
          </Button>
          {isAdmin && (
            <Button onClick={() => setCreateOpen(true)}>
              <Plus />
              New project
            </Button>
          )}
        </div>
      </div>

      {/* Table */}
      <DndProjectProvider projects={dndProjects}>
        <NoGroupDropZone />
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <button className="flex items-center gap-1" onClick={() => handleSort('name')}>
                  Name
                  <ArrowUpDown className="h-3.5 w-3.5" />
                </button>
              </TableHead>
              <TableHead>
                <button className="flex items-center gap-1" onClick={() => handleSort('type')}>
                  Type
                  <ArrowUpDown className="h-3.5 w-3.5" />
                </button>
              </TableHead>
              <TableHead>
                <button className="flex items-center gap-1" onClick={() => handleSort('pass_rate')}>
                  Pass Rate
                  <ArrowUpDown className="h-3.5 w-3.5" />
                </button>
              </TableHead>
              {isAdmin && <TableHead className="w-10" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={isAdmin ? 4 : 3} className="text-muted-foreground py-8 text-center">
                  {search ? 'No projects match your search.' : 'No projects found.'}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((project) => (
                <ProjectTableRow
                  key={project.project_id}
                  project={project}
                  isAdmin={isAdmin}
                  onDrillDown={
                    project.is_group
                      ? () => setSearchParams({ group: String(project.project_id) })
                      : undefined
                  }
                />
              ))
            )}
          </TableBody>
        </Table>
      </DndProjectProvider>

      <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}

function ProjectTableRow({
  project,
  isAdmin,
  onDrillDown,
}: {
  project: DashboardProjectEntry
  isAdmin: boolean
  onDrillDown?: () => void
}) {
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
          isOver && dropTarget && 'ring-2 ring-blue-500 scale-[1.02]',
          draggable && !onDrillDown && 'cursor-grab',
        )}
        onClick={onDrillDown}
      >
        <TableCell className="font-medium">
          <div className="flex items-center gap-2">
            {project.is_group && <Folder className="text-muted-foreground h-4 w-4 shrink-0" />}
            {project.is_group ? (
              <span>{project.slug}</span>
            ) : (
              <div className="flex flex-col">
                <NavLink
                  to={`/projects/${project.slug}`}
                  className="hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {projectLabel(project)}
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
              {rate.toFixed(0)}%
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
