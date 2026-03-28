import { useState } from 'react'
import { NavLink } from 'react-router'
import {
  AlertCircle,
  BarChart3,
  ChevronDown,
  ChevronRight,
  Clock,
  FolderOpen,
  Gauge,
  LayoutDashboard,
  Paperclip,
  Shield,
} from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useActiveProject } from '@/hooks/useActiveProject'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { projectListOptions } from '@/lib/queries'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from '@/components/ui/sidebar'
import { env } from '@/lib/env'
import type { ProjectEntry } from '@/types/api'

const navItems = [
  { label: 'Overview', path: '', icon: LayoutDashboard, end: true },
  { label: 'Analytics', path: '/analytics', icon: BarChart3, end: false },
  { label: 'Timeline', path: '/timeline', icon: Clock, end: false },
  { label: 'Known Issues', path: '/known-issues', icon: AlertCircle, end: false },
  { label: 'Attachments', path: '/attachments', icon: Paperclip, end: false },
]

interface ProjectGroup {
  parent: ProjectEntry
  children: ProjectEntry[]
}

function buildHierarchy(projects: ProjectEntry[]): {
  groups: ProjectGroup[]
  standalone: ProjectEntry[]
} {
  const byId = new Map<string, ProjectEntry>(projects.map((p) => [p.project_id, p]))
  const groups: ProjectGroup[] = []
  const standalone: ProjectEntry[] = []

  for (const project of projects) {
    // Skip child projects — they will appear under their parent
    if (project.parent_id) continue

    const childIds = project.children ?? []
    if (childIds.length > 0) {
      const children = childIds
        .map((id) => byId.get(id))
        .filter((p): p is ProjectEntry => p !== undefined)
      groups.push({ parent: project, children })
    } else {
      standalone.push(project)
    }
  }

  return { groups, standalone }
}

function ProjectGroupItem({ group }: { group: ProjectGroup }) {
  const [open, setOpen] = useState(false)

  return (
    <SidebarMenuItem>
      <SidebarMenuButton onClick={() => setOpen((v) => !v)} tooltip={group.parent.project_id}>
        <FolderOpen />
        <span className="truncate">{group.parent.project_id}</span>
        {open ? <ChevronDown size={14} className="ml-auto" /> : <ChevronRight size={14} className="ml-auto" />}
      </SidebarMenuButton>
      {open && (
        <SidebarMenuSub>
          {group.children.map((child) => (
            <SidebarMenuSubItem key={child.project_id}>
              <SidebarMenuSubButton asChild>
                <NavLink to={`/projects/${encodeURIComponent(child.project_id)}`}>
                  <span className="truncate">{child.project_id}</span>
                </NavLink>
              </SidebarMenuSubButton>
            </SidebarMenuSubItem>
          ))}
        </SidebarMenuSub>
      )}
    </SidebarMenuItem>
  )
}

export function AppSidebar() {
  const { projectId } = useActiveProject()
  const isAdmin = useAuthStore(selectIsAdmin)

  const { data: projectsResp } = useQuery(projectListOptions())

  const allProjects = projectsResp?.data ?? []
  const { groups, standalone } = buildHierarchy(allProjects)
  const hasHierarchy = groups.length > 0

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

        {/* Project hierarchy (when parent-child data is available) */}
        {hasHierarchy && (
          <SidebarGroup>
            <SidebarGroupLabel>Projects</SidebarGroupLabel>
            <SidebarMenu>
              {groups.map((group) => (
                <ProjectGroupItem key={group.parent.project_id} group={group} />
              ))}
              {standalone.map((project) => (
                <SidebarMenuItem key={project.project_id}>
                  <SidebarMenuButton asChild tooltip={project.project_id}>
                    <NavLink to={`/projects/${encodeURIComponent(project.project_id)}`}>
                      <LayoutDashboard />
                      <span className="truncate">{project.project_id}</span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroup>
        )}

        {/* Project sub-nav (active project pages) */}
        <SidebarGroup>
          {!hasHierarchy && <SidebarGroupLabel>Projects</SidebarGroupLabel>}
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
