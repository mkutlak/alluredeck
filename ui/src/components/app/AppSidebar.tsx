import { useState } from 'react'
import { NavLink, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import {
  AlertCircle,
  BarChart3,
  ChevronRight,
  Clock,
  FolderOpen,
  Gauge,
  LayoutDashboard,
  Shield,
} from 'lucide-react'
import { getProjects } from '@/api/projects'
import { useAuthStore } from '@/store/auth'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from '@/components/ui/sidebar'
import { env } from '@/lib/env'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { SearchTrigger } from '@/features/search'

const navItems = [
  { label: 'Overview', path: '', icon: LayoutDashboard, end: true },
  { label: 'Known Issues', path: '/known-issues', icon: AlertCircle, end: false },
  { label: 'Timeline', path: '/timeline', icon: Clock, end: false },
  { label: 'Analytics', path: '/analytics', icon: BarChart3, end: false },
]

export function AppSidebar() {
  const { id: projectId } = useParams<{ id: string }>()
  const [userClosed, setUserClosed] = useState(false)
  const open = !userClosed || !!projectId
  const isAdmin = useAuthStore((s) => s.isAdmin())

  const { data, isLoading } = useQuery({
    queryKey: ['projects'],
    queryFn: () => getProjects(),
    staleTime: 30_000,
  })

  const projects = data?.data ?? []

  return (
    <Sidebar collapsible="offcanvas">
      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild>
                <NavLink to="/" end>
                  <Gauge />
                  <span>Projects Dashboard</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SearchTrigger />
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {isAdmin && (
          <SidebarGroup>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton asChild>
                  <NavLink to="/admin">
                    <Shield />
                    <span>System Monitor</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        )}

        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <Collapsible open={open} onOpenChange={(next) => setUserClosed(!next)} className="group/collapsible">
                <CollapsibleTrigger asChild>
                  <SidebarMenuButton>
                    <FolderOpen />
                    <span>Projects</span>
                    <ChevronRight className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                  </SidebarMenuButton>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <SidebarMenuSub>
                    {isLoading ? (
                      <>
                        <SidebarMenuSkeleton />
                        <SidebarMenuSkeleton />
                      </>
                    ) : projects.length === 0 ? (
                      <p className="px-2 py-1 text-xs text-muted-foreground">No projects</p>
                    ) : (
                      projects.map((p) => (
                        <SidebarMenuSubItem key={p.project_id}>
                          <SidebarMenuSubButton asChild isActive={projectId === p.project_id}>
                            <NavLink to={`/projects/${p.project_id}`} end>
                              <span>{p.project_id}</span>
                            </NavLink>
                          </SidebarMenuSubButton>
                        </SidebarMenuSubItem>
                      ))
                    )}
                  </SidebarMenuSub>
                </CollapsibleContent>
              </Collapsible>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {projectId && (
          <SidebarGroup>
            <SidebarGroupLabel>{projectId}</SidebarGroupLabel>
            <SidebarMenu>
              {navItems.map(({ label, path, icon: Icon, end }) => (
                <SidebarMenuItem key={label}>
                  <SidebarMenuButton asChild>
                    <NavLink to={`/projects/${projectId}${path}`} end={end}>
                      <Icon />
                      <span>{label}</span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroup>
        )}
      </SidebarContent>
      <SidebarFooter>
        <p className="px-2 py-1 text-xs text-muted-foreground">v{env.appVersion}</p>
      </SidebarFooter>
    </Sidebar>
  )
}
