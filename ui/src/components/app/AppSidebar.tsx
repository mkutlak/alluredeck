import { NavLink } from 'react-router'
import {
  AlertCircle,
  BarChart3,
  Bell,
  Bug,
  Clock,
  Gauge,
  KeyRound,
  LayoutDashboard,
  Paperclip,
  Shield,
  UserCircle,
  UsersRound,
} from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useActiveProject } from '@/hooks/useActiveProject'
import { useAuthStore, selectIsAdmin, selectIsEditor } from '@/store/auth'
import { projectIndexOptions } from '@/lib/queries'
import { resolveProjectFromParam } from '@/lib/resolveProject'
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
  { label: 'Analytics', path: '/analytics', icon: BarChart3, end: false },
  { label: 'Defects', path: '/defects', icon: Bug, end: false },
  { label: 'Timeline', path: '/timeline', icon: Clock, end: false },
  { label: 'Known Issues', path: '/known-issues', icon: AlertCircle, end: false },
  { label: 'Attachments', path: '/attachments', icon: Paperclip, end: false },
]

export function AppSidebar() {
  const { projectId } = useActiveProject()
  const isAdmin = useAuthStore(selectIsAdmin)
  const isEditor = useAuthStore(selectIsEditor)

  const { data: projectsResp } = useQuery(projectIndexOptions())
  const allProjects = projectsResp?.data ?? []

  return (
    <Sidebar collapsible="icon" className="h-full">
      <SidebarContent>
        {/* Home */}
        <SidebarGroup>
          <SidebarGroupLabel>Home</SidebarGroupLabel>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Projects">
                <NavLink to="/" end>
                  <Gauge />
                  <span>Projects</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {/* Project sub-nav (active project pages) */}
        {projectId && (
          <SidebarGroup>
            <SidebarGroupLabel>Project</SidebarGroupLabel>
            <SidebarMenu>
              {(() => {
                const currentProject = resolveProjectFromParam(projectId ?? undefined, allProjects)
                const isParent = (currentProject?.children?.length ?? 0) > 0
                const parentHiddenTabs = ['Timeline', 'Known Issues', 'Attachments']
                const visibleNavItems = isParent
                  ? navItems.filter((item) => !parentHiddenTabs.includes(item.label))
                  : navItems

                return visibleNavItems.map(({ label, path, icon: Icon, end }) => (
                  <SidebarMenuItem key={label}>
                    <SidebarMenuButton asChild tooltip={label}>
                      <NavLink
                        to={`/projects/${projectId}${path}`}
                        end={end}
                        data-testid={`sidebar-nav-${label.toLowerCase().replace(/\s+/g, '-')}`}
                      >
                        <Icon />
                        <span>{label}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))
              })()}
            </SidebarMenu>
          </SidebarGroup>
        )}

        {/* Administration — anchored to bottom */}
        <SidebarGroup className="mt-auto">
          <SidebarGroupLabel>Administration</SidebarGroupLabel>
          <SidebarMenu>
            {isAdmin && (
              <SidebarMenuItem>
                <SidebarMenuButton asChild tooltip="System Monitor">
                  <NavLink to="/admin">
                    <Shield />
                    <span>System Monitor</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="API Keys">
                <NavLink to="/settings/api-keys">
                  <KeyRound />
                  <span>API Keys</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
            {isEditor && (
              <SidebarMenuItem>
                <SidebarMenuButton asChild tooltip="Webhooks">
                  <NavLink to="/settings/webhooks">
                    <Bell />
                    <span>Webhooks</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
            {isAdmin && (
              <SidebarMenuItem>
                <SidebarMenuButton asChild tooltip="Users">
                  <NavLink to="/settings/users">
                    <UsersRound />
                    <span>Users</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Profile">
                <NavLink to="/settings/profile">
                  <UserCircle />
                  <span>Profile</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <p className="text-muted-foreground px-2 py-1 text-xs">v{env.appVersion}</p>
      </SidebarFooter>
    </Sidebar>
  )
}
