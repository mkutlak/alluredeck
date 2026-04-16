import { Button } from '@/components/ui/button'
import type { ViewMode } from './sort'

interface DashboardViewToggleProps {
  viewMode: ViewMode
  onChange: (mode: ViewMode) => void
}

export function DashboardViewToggle({ viewMode, onChange }: DashboardViewToggleProps) {
  return (
    <div className="flex rounded-md border">
      <Button
        size="sm"
        variant="ghost"
        className={`rounded-r-none border-r px-3 ${viewMode === 'grouped' ? 'bg-muted font-semibold' : ''}`}
        onClick={() => onChange('grouped')}
      >
        Grouped
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className={`rounded-l-none px-3 ${viewMode === 'all' ? 'bg-muted font-semibold' : ''}`}
        onClick={() => onChange('all')}
      >
        All
      </Button>
    </div>
  )
}
