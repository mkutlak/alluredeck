import { useState } from 'react'
import { ChevronDown, Layers } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'

import { projectIndexOptions } from '@/lib/queries'
import { formatProjectLabel } from '@/lib/projectLabel'
import { useUIStore } from '@/store/ui'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export function GroupFilter() {
  const [open, setOpen] = useState(false)
  const runsFeedGroupIds = useUIStore((s) => s.runsFeedGroupIds)
  const setRunsFeedGroupIds = useUIStore((s) => s.setRunsFeedGroupIds)

  const { data } = useQuery(projectIndexOptions())
  const projects = data?.data ?? []
  const groups = projects.filter((p) => (p.children?.length ?? 0) > 0)

  const toggle = (id: number) => {
    setRunsFeedGroupIds(
      runsFeedGroupIds.includes(id)
        ? runsFeedGroupIds.filter((g) => g !== id)
        : [...runsFeedGroupIds, id],
    )
  }

  const label =
    runsFeedGroupIds.length === 0
      ? 'All groups'
      : `${runsFeedGroupIds.length} group${runsFeedGroupIds.length === 1 ? '' : 's'} selected`

  if (groups.length === 0) return null

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5" aria-label="Filter by group">
          <Layers size={14} />
          {label}
          <ChevronDown size={14} />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-2">
        <div className="space-y-1">
          {groups.map((g) => {
            const checkboxId = `group-filter-${g.project_id}`
            return (
              <label
                key={g.project_id}
                htmlFor={checkboxId}
                className="hover:bg-accent flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm"
              >
                <Checkbox
                  id={checkboxId}
                  checked={runsFeedGroupIds.includes(g.project_id)}
                  onCheckedChange={() => toggle(g.project_id)}
                />
                {formatProjectLabel(g, projects)}
              </label>
            )
          })}
        </div>
      </PopoverContent>
    </Popover>
  )
}
