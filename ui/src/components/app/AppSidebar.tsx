import { NavLink } from 'react-router'
import { Bell, Gauge, Inbox, KeyRound, Shield, UsersRound } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore, selectIsAdmin, selectIsEditor } from '@/store/auth'
import { getConfig } from '@/api/system'
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

export function AppSidebar() {
  const isAdmin = useAuthStore(selectIsAdmin)
  const isEditor = useAuthStore(selectIsEditor)

  const { data: configResp } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
    enabled: isAdmin,
  })
  const configData = configResp?.data

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
            {isAdmin && configData?.mcp_enabled && (
              <SidebarMenuItem>
                <SidebarMenuButton asChild tooltip="Pending Proposals">
                  <NavLink to="/admin/proposals">
                    <Inbox />
                    <span>Pending Proposals</span>
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
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <p className="text-muted-foreground px-2 py-1 text-xs">v{env.appVersion}</p>
      </SidebarFooter>
    </Sidebar>
  )
}
