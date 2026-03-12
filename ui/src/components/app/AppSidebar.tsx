import { NavLink } from 'react-router'
import { AlertCircle, BarChart3, Clock, Gauge, LayoutDashboard, Shield } from 'lucide-react'
import { useActiveProject } from '@/hooks/useActiveProject'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { env } from '@/lib/env'

const navItems = [
  { label: 'Overview', path: '', icon: LayoutDashboard, end: true },
  { label: 'Known Issues', path: '/known-issues', icon: AlertCircle, end: false },
  { label: 'Timeline', path: '/timeline', icon: Clock, end: false },
  { label: 'Analytics', path: '/analytics', icon: BarChart3, end: false },
]

export function AppSidebar() {
  const { projectId } = useActiveProject()
  const isAdmin = useAuthStore(selectIsAdmin)

  return (
    <Sidebar collapsible="icon">
      <SidebarContent>
        {/* Home */}
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Projects Dashboard">
                <NavLink to="/" end>
                  <Gauge />
                  <span>Projects Dashboard</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {/* Project sub-nav */}
        <SidebarGroup>
          <SidebarGroupLabel>Projects</SidebarGroupLabel>
          {projectId && (
            <SidebarMenu>
              {navItems.map(({ label, path, icon: Icon, end }) => (
                <SidebarMenuItem key={label}>
                  <SidebarMenuButton asChild tooltip={label}>
                    <NavLink to={`/projects/${projectId}${path}`} end={end}>
                      <Icon />
                      <span>{label}</span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          )}
        </SidebarGroup>

        {/* Administration (admin only) */}
        {isAdmin && (
          <SidebarGroup>
            <SidebarGroupLabel>Administration</SidebarGroupLabel>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton asChild tooltip="System Monitor">
                  <NavLink to="/admin">
                    <Shield />
                    <span>System Monitor</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        )}
      </SidebarContent>
      <SidebarFooter>
        <p className="text-muted-foreground px-2 py-1 text-xs">v{env.appVersion}</p>
      </SidebarFooter>
    </Sidebar>
  )
}
