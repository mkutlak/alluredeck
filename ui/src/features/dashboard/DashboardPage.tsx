import { useState, useMemo } from 'react'
import { useSearchParams } from 'react-router'
import { Plus } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { dashboardOptions } from '@/lib/queries'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { CreateProjectDialog } from '@/features/projects/CreateProjectDialog'
import type { DndProject } from '@/features/projects/hooks/useProjectDnd'
import type { DashboardProjectEntry } from '@/types/api'
import { DashboardHeader } from './components/DashboardHeader'
import { DashboardTable } from './components/DashboardTable'
import { compareRows } from './components/sort'
import type { SortDir, SortField, ViewMode } from './components/sort'

export function DashboardPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [viewMode, setViewMode] = useState<ViewMode>('grouped')
  const [sortField, setSortField] = useState<SortField>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [searchParams, setSearchParams] = useSearchParams()
  const groupIdRaw = searchParams.get('group')
  const groupId = groupIdRaw != null ? parseInt(groupIdRaw, 10) : null
  const isAdmin = useAuthStore(selectIsAdmin)

  const { data, isLoading, isFetching, isError, refetch } = useQuery(dashboardOptions())

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortField(field)
      setSortDir('asc')
    }
  }

  const rows = useMemo(() => {
    if (!data) return []
    const { projects } = data

    // Drill-down: show only children of the selected group
    if (groupId != null && !isNaN(groupId)) {
      const group = projects.find((p) => p.project_id === groupId && p.is_group)
      const children = group?.children ?? []
      const filtered = search
        ? children.filter((c) => {
            const q = search.toLowerCase()
            return c.slug.toLowerCase().includes(q) || (c.display_name ?? '').toLowerCase().includes(q)
          })
        : children
      return [...filtered].sort((a, b) => compareRows(a, b, sortField, sortDir))
    }

    // All mode: flatten everything.
    // Dedup by slug (not project_id) because the URL `/projects/:slug` resolves
    // to a single project via GetProjectBySlugAny — two entries with the same
    // slug would both link to the same page and break strict-mode locators.
    // Prefer standalone entries over same-slug children (matches URL resolution).
    if (viewMode === 'all') {
      const flat: DashboardProjectEntry[] = []
      const seen = new Set<string>()
      for (const p of projects) {
        if (!p.is_group && !seen.has(p.slug)) {
          seen.add(p.slug)
          flat.push(p)
        }
      }
      for (const p of projects) {
        if (p.is_group && p.children) {
          for (const c of p.children) {
            if (!seen.has(c.slug)) {
              seen.add(c.slug)
              flat.push(c)
            }
          }
        }
      }
      const filtered = search
        ? flat.filter((p) => {
            const q = search.toLowerCase()
            return p.slug.toLowerCase().includes(q) || (p.display_name ?? '').toLowerCase().includes(q)
          })
        : flat
      return [...filtered].sort((a, b) => compareRows(a, b, sortField, sortDir))
    }

    // Grouped mode: groups first, then ungrouped projects
    const filtered = search
      ? projects.filter((p) => {
          const q = search.toLowerCase()
          const nameMatch = p.slug.toLowerCase().includes(q) || (p.display_name ?? '').toLowerCase().includes(q)
          if (nameMatch) return true
          if (p.is_group && p.children) {
            return p.children.some((c) => {
              const q = search.toLowerCase()
              return c.slug.toLowerCase().includes(q) || (c.display_name ?? '').toLowerCase().includes(q)
            })
          }
          return false
        })
      : projects

    const groups = filtered.filter((p) => p.is_group)
    const standalone = filtered.filter((p) => !p.is_group)
    return [
      ...groups.sort((a, b) => compareRows(a, b, sortField, sortDir)),
      ...standalone.sort((a, b) => compareRows(a, b, sortField, sortDir)),
    ]
  }, [data, groupId, viewMode, search, sortField, sortDir])

  const allFlatProjects = useMemo(() => {
    if (!data) return undefined
    const flat: DashboardProjectEntry[] = []
    for (const p of data.projects) {
      flat.push(p)
      if (p.children) {
        for (const c of p.children) flat.push(c)
      }
    }
    return flat
  }, [data])

  const dndProjects: DndProject[] = useMemo(() => {
    if (!data) return []
    const items: DndProject[] = []
    for (const p of data.projects) {
      items.push({
        slug: p.slug,
        projectId: p.project_id,
        parentId: null, // top-level entries in dashboard
        hasChildren: !!(p.is_group && p.children?.length),
      })
      if (p.children) {
        for (const child of p.children) {
          items.push({
            slug: child.slug,
            projectId: child.project_id,
            parentId: p.project_id,
            hasChildren: false,
          })
        }
      }
    }
    return items
  }, [data])

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="mb-6">
          <h1 className="text-2xl font-bold">Projects</h1>
        </div>
        <div className="space-y-2">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-10 animate-pulse rounded-md" />
          ))}
        </div>
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">Failed to load dashboard.</p>
        <Button onClick={() => refetch()}>Retry</Button>
      </div>
    )
  }

  if (!data || data.projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium">No projects yet</p>
        <p className="text-muted-foreground text-sm">
          {isAdmin ? 'Create a project to get started.' : 'Ask an admin to create a project.'}
        </p>
        {isAdmin && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus />
            Create first project
          </Button>
        )}
        <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
      </div>
    )
  }

  return (
    <div className="p-6">
      <DashboardHeader
        projects={data.projects}
        groupId={groupId}
        onClearGroup={() => setSearchParams({})}
        search={search}
        onSearchChange={setSearch}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        isFetching={isFetching}
        onRefetch={() => refetch()}
        isAdmin={isAdmin}
        onCreate={() => setCreateOpen(true)}
      />

      <DashboardTable
        rows={rows}
        dndProjects={dndProjects}
        isAdmin={isAdmin}
        search={search}
        onSort={handleSort}
        onDrillDown={(projectId) => setSearchParams({ group: String(projectId) })}
        allProjects={allFlatProjects}
      />

      <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}
