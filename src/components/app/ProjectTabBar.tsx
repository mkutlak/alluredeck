import { NavLink, useParams } from 'react-router'
import { cn } from '@/lib/utils'

const tabClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    'px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px',
    isActive
      ? 'border-primary text-foreground'
      : 'border-transparent text-muted-foreground hover:text-foreground hover:border-muted-foreground',
  )

export function ProjectTabBar() {
  const { id: projectId } = useParams<{ id: string }>()
  if (!projectId) return null

  const base = `/projects/${projectId}`

  return (
    <div className="flex shrink-0 overflow-x-auto border-b bg-background">
      <nav className="flex items-end px-4">
        <NavLink to={base} end className={tabClass}>
          Overview
        </NavLink>
        <NavLink to={`${base}/analytics`} className={tabClass}>
          Analytics
        </NavLink>
      </nav>
    </div>
  )
}
