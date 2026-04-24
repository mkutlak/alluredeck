import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Bell, ChevronsUpDown, Plus } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { projectIndexOptions } from '@/lib/queries'
import { formatProjectLabel } from '@/lib/projectLabel'
import { WebhookList } from './components/WebhookList'

export function WebhooksPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const projectId = searchParams.get('project') ?? ''

  const [createOpen, setCreateOpen] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)

  const { data } = useQuery(projectIndexOptions())
  const projects = data?.data ?? []

  const selectedProject = projects.find((p) => String(p.project_id) === projectId)
  const pickerLabel = selectedProject
    ? formatProjectLabel(selectedProject, projects)
    : 'Select a project...'

  function handleSelect(id: number) {
    setSearchParams({ project: String(id) })
    setPickerOpen(false)
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Webhooks</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Manage webhook notifications for project events.
          </p>
        </div>
        {projectId && (
          <Button onClick={() => setCreateOpen(true)} aria-label="Add Webhook">
            <Plus size={16} className="mr-1" />
            Add Webhook
          </Button>
        )}
      </div>

      <Popover open={pickerOpen} onOpenChange={setPickerOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="button"
            aria-label={pickerLabel}
            className="flex items-center gap-1"
          >
            <span>{pickerLabel}</span>
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

      {projectId ? (
        <WebhookList
          projectId={projectId}
          createOpen={createOpen}
          onCreateOpenChange={setCreateOpen}
        />
      ) : (
        <div className="flex items-center gap-2 rounded-md border border-dashed p-6">
          <Bell className="text-muted-foreground" size={20} />
          <p className="text-muted-foreground text-sm">
            Select a project to manage its webhooks.
          </p>
        </div>
      )}
    </div>
  )
}
