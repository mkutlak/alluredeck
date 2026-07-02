import { ChevronRight, Plus, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/app/PageHeader'
import { FilterBar } from '@/components/app/FilterBar'
import { SearchInput } from '@/components/ui/SearchInput'
import { Segmented } from '@/components/ui/segmented'
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

  const title = drilledDown ? (
    <span className="inline-flex items-center gap-2">
      <button onClick={onClearGroup} className="text-muted-foreground hover:underline">
        Projects
      </button>
      <ChevronRight className="text-muted-foreground h-5 w-5" />
      {projects.find((p) => p.project_id === groupId)?.slug ?? String(groupId)}
    </span>
  ) : (
    'Projects'
  )

  return (
    <PageHeader
      title={title}
      titleVariant="sans"
      subtitle={
        !drilledDown ? `${projects.length} project${projects.length !== 1 ? 's' : ''}` : undefined
      }
      actions={
        <>
          <Button variant="outline" size="icon" onClick={onRefetch} aria-label="Refresh">
            <RefreshCw className={isFetching ? 'animate-spin' : ''} />
          </Button>
          {isAdmin && (
            <Button onClick={onCreate}>
              <Plus />
              New project
            </Button>
          )}
        </>
      }
      toolbar={
        <FilterBar
          search={
            <SearchInput
              placeholder="Search..."
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              aria-label="Search projects"
              className="w-48"
            />
          }
          filters={
            !drilledDown ? (
              <Segmented
                aria-label="View mode"
                value={viewMode}
                onValueChange={onViewModeChange}
                options={[
                  { value: 'grouped', label: 'Grouped' },
                  { value: 'all', label: 'All' },
                ]}
              />
            ) : undefined
          }
        />
      }
    />
  )
}
