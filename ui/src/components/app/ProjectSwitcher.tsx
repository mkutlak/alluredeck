import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronsUpDown } from 'lucide-react'
import { projectListOptions } from '@/lib/queries'
import { useActiveProject } from '@/hooks/useActiveProject'
import { useProjectDisplay } from '@/features/projects/useProjectDisplay'
import { formatProjectLabel } from '@/lib/projectLabel'
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
  const [open, setOpen] = useState(false)

  const { data } = useQuery(projectListOptions())
  const projects = data?.data ?? []
  const display = useProjectDisplay(projectId ?? undefined)
  const label = display || (projectId ? '' : 'Select a project...')

  function handleSelect(id: number) {
    const value = String(id)
    setLastProjectId(value)
    setOpen(false)
    navigate(`/projects/${value}`)
  }

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
      <PopoverContent className="w-56 p-0" align="start">
        <Command>
          <CommandInput placeholder="Search project..." />
          <CommandList>
            <CommandEmpty>No projects found.</CommandEmpty>
            <CommandGroup>
              {projects.map((p) => {
                const label = formatProjectLabel(p, projects)
                return (
                  <CommandItem
                    key={p.project_id}
                    value={label}
                    onSelect={() => handleSelect(p.project_id)}
                  >
                    {label}
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
