import { NavLink, useParams } from 'react-router'
import { LayoutDashboard, AlertCircle, Clock, BarChart3, Gauge } from 'lucide-react'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { ProjectSwitcher } from './ProjectSwitcher'
import { SearchTrigger } from '@/features/search'

const navItems = [
  { label: 'Overview', path: '', icon: LayoutDashboard, end: true },
  { label: 'Known Issues', path: '/known-issues', icon: AlertCircle, end: false },
  { label: 'Timeline', path: '/timeline', icon: Clock, end: false },
  { label: 'Analytics', path: '/analytics', icon: BarChart3, end: false },
]

export function AppSidebar() {
  const { id: projectId } = useParams<{ id: string }>()

  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader>
        <ProjectSwitcher />
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild>
                <NavLink to="/dashboard" end>
                  <Gauge />
                  <span>Dashboard</span>
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

        {projectId && (
          <SidebarGroup>
            <SidebarGroupLabel>Navigation</SidebarGroupLabel>
            <SidebarMenu>
              {navItems.map(({ label, path, icon: Icon, end }) => {
                const to = `/projects/${projectId}${path}`
                return (
                  <SidebarMenuItem key={label}>
                    <SidebarMenuButton asChild>
                      <NavLink to={to} end={end}>
                        <Icon />
                        <span>{label}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroup>
        )}
      </SidebarContent>
    </Sidebar>
  )
}
