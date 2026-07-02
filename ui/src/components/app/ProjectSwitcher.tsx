import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Check, ChevronsUpDown, FileText, Folder, Star } from 'lucide-react'
import { projectIndexOptions } from '@/lib/queries'
import { useActiveProject } from '@/hooks/useActiveProject'
import { useProjectDisplay } from '@/features/projects/useProjectDisplay'
import { formatProjectLabel, splitProjectLabel } from '@/lib/projectLabel'
import { useUIStore } from '@/store/ui'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'

export function ProjectSwitcher() {
  const { projectId } = useActiveProject()
  const navigate = useNavigate()
  const setLastProjectId = useUIStore((s) => s.setLastProjectId)
  const pinnedProjectIds = useUIStore((s) => s.pinnedProjectIds)
  const recentProjectIds = useUIStore((s) => s.recentProjectIds)
  const pinProject = useUIStore((s) => s.pinProject)
  const unpinProject = useUIStore((s) => s.unpinProject)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const { data } = useQuery(projectIndexOptions())
  const projects = data?.data ?? []
  const display = useProjectDisplay(projectId ?? undefined)
  const label = display || (projectId ? '' : 'Select a project...')

  const isGroup = (id: number) => {
    const p = projects.find((x) => x.project_id === id)
    return p != null && Array.isArray(p.children) && p.children.length > 0
  }

  const leafProjects = projects.filter((p) => !isGroup(p.project_id))

  function handleSelect(id: number) {
    const value = String(id)
    setLastProjectId(value)
    setOpen(false)
    setSearch('')
    const lastTab = useUIStore.getState().lastTabPerProject[value]
    navigate(`/projects/${value}${lastTab ? '/' + lastTab : ''}`)
  }

  function handlePinToggle(e: React.MouseEvent, id: number) {
    e.stopPropagation()
    if (pinnedProjectIds.includes(id)) {
      unpinProject(id)
    } else {
      pinProject(id)
    }
  }

  function renderProjectRow(id: number, indented = false, showParent = false) {
    const project = projects.find((p) => p.project_id === id)
    if (!project) return null
    const itemLabel = formatProjectLabel(project, projects)
    const { name, parentName } = splitProjectLabel(project, projects)
    const isPinned = pinnedProjectIds.includes(id)
    const isActive = projectId != null && String(id) === String(projectId)
    return (
      <CommandItem
        key={id}
        value={itemLabel}
        onSelect={() => handleSelect(id)}
        className={indented ? 'pl-6' : undefined}
      >
        <FileText className="mr-2 h-4 w-4 shrink-0" />
        <span className="flex-1 truncate">{name}</span>
        {showParent && parentName && (
          <span className="text-muted-foreground ml-auto shrink-0 text-xs">{parentName}</span>
        )}
        {isActive && <Check className="ml-1 h-4 w-4 shrink-0" />}
        <button
          aria-label={isPinned ? `Unpin ${itemLabel}` : `Pin ${itemLabel}`}
          className={`ml-1 shrink-0 opacity-0 group-hover/item:opacity-100 ${isPinned ? 'opacity-100' : ''}`}
          onClick={(e) => handlePinToggle(e, id)}
        >
          <Star
            className={`h-3.5 w-3.5 ${isPinned ? 'text-warning fill-current' : 'text-muted-foreground'}`}
          />
        </button>
      </CommandItem>
    )
  }

  // Hierarchical "All Projects" list: group headers (non-selectable) + indented children + flat standalones
  function renderAllProjects() {
    const groupProjects = projects.filter((p) => isGroup(p.project_id))
    const childIds = new Set(groupProjects.flatMap((p) => p.children ?? []))
    const standalones = leafProjects.filter(
      (p) => p.parent_id == null && !childIds.has(p.project_id),
    )

    return (
      <>
        {groupProjects.map((group) => {
          const isActive = projectId != null && String(group.project_id) === String(projectId)
          return (
            <div key={group.project_id}>
              <CommandItem
                value={`__group__${group.project_id}`}
                onSelect={() => handleSelect(group.project_id)}
              >
                <Folder className="mr-2 h-4 w-4 shrink-0" />
                <span className="flex-1 truncate font-medium">
                  {formatProjectLabel(group, projects)}
                </span>
                {isActive && <Check className="ml-1 h-4 w-4 shrink-0" />}
              </CommandItem>
              {(group.children ?? []).map((childId) => renderProjectRow(childId, true, false))}
            </div>
          )
        })}
        {standalones.map((p) => renderProjectRow(p.project_id, false, true))}
      </>
    )
  }

  const recentProjects = recentProjectIds
    .map((id) => projects.find((p) => p.project_id === id))
    .filter((p): p is NonNullable<typeof p> => p != null)

  const pinnedProjects = pinnedProjectIds
    .map((id) => projects.find((p) => p.project_id === id))
    .filter((p): p is NonNullable<typeof p> => p != null)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          role="button"
          aria-label={label || 'Select a project...'}
          className="flex h-8 items-center gap-1 px-2 text-sm"
        >
          <span>{label}</span>
          <ChevronsUpDown className="h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 p-0" align="start">
        <Command>
          <CommandInput placeholder="Search project..." value={search} onValueChange={setSearch} />
          <CommandList>
            <CommandEmpty>No projects found.</CommandEmpty>
            {search === '' ? (
              <>
                {recentProjects.length > 0 && (
                  <CommandGroup heading="Recents">
                    {recentProjects.map((p) => renderProjectRow(p.project_id, false, true))}
                  </CommandGroup>
                )}
                {pinnedProjects.length > 0 && (
                  <CommandGroup heading="Pinned">
                    {pinnedProjects.map((p) => renderProjectRow(p.project_id, false, true))}
                  </CommandGroup>
                )}
                <CommandGroup heading="All Projects">{renderAllProjects()}</CommandGroup>
              </>
            ) : (
              <CommandGroup>
                {projects.map((p) => {
                  const itemLabel = formatProjectLabel(p, projects)
                  const { name, parentName } = splitProjectLabel(p, projects)
                  const group = isGroup(p.project_id)
                  const isActive = projectId != null && String(p.project_id) === String(projectId)
                  return (
                    <CommandItem
                      key={p.project_id}
                      value={itemLabel}
                      onSelect={() => handleSelect(p.project_id)}
                    >
                      {group ? (
                        <Folder className="mr-2 h-4 w-4 shrink-0" />
                      ) : (
                        <FileText className="mr-2 h-4 w-4 shrink-0" />
                      )}
                      <span className={`flex-1 truncate ${group ? 'font-medium' : ''}`}>
                        {name}
                      </span>
                      {!group && parentName && (
                        <span className="text-muted-foreground ml-auto shrink-0 text-xs">
                          {parentName}
                        </span>
                      )}
                      {isActive && <Check className="ml-1 h-4 w-4 shrink-0" />}
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
