import { NavLink, useParams } from 'react-router-dom'
import { LayoutDashboard, FolderOpen, ChevronLeft, ChevronRight } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { cn } from '@/lib/utils'
import { useUIStore } from '@/store/ui'
import { getProjects } from '@/api/projects'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

export function Sidebar() {
  const collapsed = useUIStore((s) => s.sidebarCollapsed)
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)
  const { id: activeProjectId } = useParams<{ id: string }>()

  const { data, isLoading } = useQuery({
    queryKey: ['projects'],
    queryFn: getProjects,
    staleTime: 30_000,
  })

  const projects = data ? Object.values(data.data) : []

  return (
    <aside
      className={cn(
        'relative flex h-full flex-col border-r bg-background transition-[width] duration-200',
        collapsed ? 'w-14' : 'w-60',
      )}
    >
      {/* Nav items */}
      <nav className="flex flex-1 flex-col gap-1 overflow-y-auto p-2">
        <NavItem to="/" icon={<LayoutDashboard size={16} />} label="Projects" collapsed={collapsed} />

        {/* Project list */}
        {!collapsed && projects.length > 0 && (
          <div className="mt-2">
            <p className="mb-1 px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Projects
            </p>
            {isLoading
              ? Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="mx-2 my-1 h-7 w-full" />
                ))
              : projects.map((p) => (
                  <NavLink
                    key={p.project_id}
                    to={`/projects/${p.project_id}`}
                    className={({ isActive }) =>
                      cn(
                        'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent',
                        isActive || activeProjectId === p.project_id
                          ? 'bg-accent font-medium text-accent-foreground'
                          : 'text-muted-foreground',
                      )
                    }
                  >
                    <FolderOpen size={14} className="shrink-0" />
                    <span className="truncate">{p.project_id}</span>
                  </NavLink>
                ))}
          </div>
        )}

        {collapsed && projects.length > 0 && (
          <div className="mt-2 flex flex-col gap-1">
            {projects.map((p) => (
              <Tooltip key={p.project_id} delayDuration={300}>
                <TooltipTrigger asChild>
                  <NavLink
                    to={`/projects/${p.project_id}`}
                    className={({ isActive }) =>
                      cn(
                        'flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
                        isActive && 'bg-accent text-accent-foreground',
                      )
                    }
                  >
                    <FolderOpen size={14} />
                  </NavLink>
                </TooltipTrigger>
                <TooltipContent side="right">{p.project_id}</TooltipContent>
              </Tooltip>
            ))}
          </div>
        )}
      </nav>

      {/* Collapse toggle */}
      <div className="border-t p-2">
        <Button
          variant="ghost"
          size="icon"
          onClick={toggleSidebar}
          className="h-7 w-full"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <ChevronRight size={14} /> : <ChevronLeft size={14} />}
        </Button>
      </div>
    </aside>
  )
}

// ---------------------------------------------------------------------------
// Internal helper
// ---------------------------------------------------------------------------
interface NavItemProps {
  to: string
  icon: React.ReactNode
  label: string
  collapsed: boolean
}

function NavItem({ to, icon, label, collapsed }: NavItemProps) {
  const item = (
    <NavLink
      to={to}
      end
      className={({ isActive }) =>
        cn(
          'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent',
          isActive ? 'bg-accent font-medium text-accent-foreground' : 'text-muted-foreground',
          collapsed && 'justify-center',
        )
      }
    >
      {icon}
      {!collapsed && <span>{label}</span>}
    </NavLink>
  )

  if (!collapsed) return item

  return (
    <Tooltip delayDuration={300}>
      <TooltipTrigger asChild>{item}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}
