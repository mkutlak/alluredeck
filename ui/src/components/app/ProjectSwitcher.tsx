import { useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronsUpDown, FolderOpen, LayoutDashboard } from 'lucide-react'
import { getProjects } from '@/api/projects'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { SidebarMenuButton } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

export function ProjectSwitcher() {
  const [open, setOpen] = useState(false)
  const { id: activeProjectId } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const { data } = useQuery({
    queryKey: ['projects'],
    queryFn: () => getProjects(),
    staleTime: 30_000,
  })

  const projects = data ? Object.values(data.data) : []

  const handleSelect = (projectId: string | null) => {
    setOpen(false)
    if (projectId === null) {
      navigate('/')
    } else {
      navigate(`/projects/${projectId}`)
    }
  }

  const label = activeProjectId ?? 'Select project…'

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <SidebarMenuButton
          size="lg"
          className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
        >
          {activeProjectId ? (
            <FolderOpen className="size-4 shrink-0" />
          ) : (
            <LayoutDashboard className="size-4 shrink-0" />
          )}
          <span className="flex-1 truncate text-left text-sm font-medium">{label}</span>
          <ChevronsUpDown className="ml-auto size-4 shrink-0 opacity-50" />
        </SidebarMenuButton>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search projects…" />
          <CommandList>
            <CommandEmpty>No projects found.</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value="all-projects"
                onSelect={() => handleSelect(null)}
                className={cn(!activeProjectId && 'font-medium')}
              >
                <LayoutDashboard className="size-4 shrink-0 text-muted-foreground" />
                All projects
              </CommandItem>
              {projects.map((p) => (
                <CommandItem
                  key={p.project_id}
                  value={p.project_id}
                  onSelect={() => handleSelect(p.project_id)}
                  className={cn(activeProjectId === p.project_id && 'font-medium')}
                >
                  <FolderOpen className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">{p.project_id}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
