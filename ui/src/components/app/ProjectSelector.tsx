import { useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronsUpDown, FolderOpen, LayoutDashboard } from 'lucide-react'
import { getProjects } from '@/api/projects'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

export function ProjectSelector() {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const { id: activeProjectId } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const { data } = useQuery({
    queryKey: ['projects'],
    queryFn: getProjects,
    staleTime: 30_000,
  })

  const projects = data ? Object.values(data.data) : []
  const filtered = search
    ? projects.filter((p) => p.project_id.toLowerCase().includes(search.toLowerCase()))
    : projects

  const handleSelect = (projectId: string | null) => {
    setOpen(false)
    setSearch('')
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
        <Button
          variant="outline"
          size="sm"
          role="combobox"
          aria-expanded={open}
          className="w-52 justify-between font-normal"
        >
          <span className="truncate">{label}</span>
          <ChevronsUpDown size={14} className="ml-2 shrink-0 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-52 p-0" align="start">
        <div className="border-b p-2">
          <Input
            placeholder="Search projects…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-7 text-sm"
            autoFocus
          />
        </div>
        <div className="max-h-64 overflow-y-auto py-1">
          <button
            className={cn(
              'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors hover:bg-accent',
              activeProjectId === undefined && 'bg-accent font-medium',
            )}
            onClick={() => handleSelect(null)}
          >
            <LayoutDashboard size={14} className="shrink-0 text-muted-foreground" />
            All projects
          </button>
          {filtered.map((p) => (
            <button
              key={p.project_id}
              className={cn(
                'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors hover:bg-accent',
                activeProjectId === p.project_id && 'bg-accent font-medium',
              )}
              onClick={() => handleSelect(p.project_id)}
            >
              <FolderOpen size={14} className="shrink-0 text-muted-foreground" />
              <span className="truncate">{p.project_id}</span>
            </button>
          ))}
          {filtered.length === 0 && (
            <p className="px-3 py-4 text-center text-sm text-muted-foreground">No projects found</p>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
