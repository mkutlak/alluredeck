import { ChevronRight, Plus, RefreshCw, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DashboardViewToggle } from './DashboardViewToggle'
import type { ViewMode } from './sort'
import type { DashboardProjectEntry } from '@/types/api'

interface DashboardHeaderProps {
  projects: DashboardProjectEntry[]
  groupId: number | null
  onClearGroup: () => void
  search: string
  onSearchChange: (value: string) => void
  viewMode: ViewMode
  onViewModeChange: (mode: ViewMode) => void
  isFetching: boolean
  onRefetch: () => void
  isAdmin: boolean
  onCreate: () => void
}

export function DashboardHeader({
  projects,
  groupId,
  onClearGroup,
  search,
  onSearchChange,
  viewMode,
  onViewModeChange,
  isFetching,
  onRefetch,
  isAdmin,
  onCreate,
}: DashboardHeaderProps) {
  const drilledDown = groupId != null && !isNaN(groupId)

  return (
    <div className="mb-6 flex items-center justify-between gap-4">
      <div className="flex items-center gap-2">
        {drilledDown ? (
          <>
            <button
              onClick={onClearGroup}
              className="text-muted-foreground text-2xl font-bold hover:underline"
            >
              Projects
            </button>
            <ChevronRight className="text-muted-foreground h-5 w-5" />
            <h1 className="text-2xl font-bold">
              {projects.find((p) => p.project_id === groupId)?.slug ?? String(groupId)}
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
            onChange={(e) => onSearchChange(e.target.value)}
            className="w-48 pl-8"
          />
        </div>
        {!drilledDown && <DashboardViewToggle viewMode={viewMode} onChange={onViewModeChange} />}
        <Button variant="outline" size="icon" onClick={onRefetch} aria-label="Refresh">
          <RefreshCw className={isFetching ? 'animate-spin' : ''} />
        </Button>
        {isAdmin && (
          <Button onClick={onCreate}>
            <Plus />
            New project
          </Button>
        )}
      </div>
    </div>
  )
}
