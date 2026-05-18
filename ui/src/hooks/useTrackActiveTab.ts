import { useEffect } from 'react'
import { useLocation } from 'react-router'
import { useUIStore } from '@/store/ui'

const TOP_LEVEL_TABS = new Set(['', 'analytics', 'defects', 'timeline', 'known-issues', 'attachments'])

// Matches /projects/:id and optionally one more path segment (the tab).
// Anything deeper (e.g. /projects/:id/reports/123) is intentionally ignored.
const PROJECT_ROUTE_RE = /^\/projects\/[^/]+(?:\/([^/]*))?$/

export function useTrackActiveTab(projectId: string | null): void {
  const { pathname } = useLocation()
  const setLastTabForProject = useUIStore((s) => s.setLastTabForProject)

  useEffect(() => {
    if (projectId === null) return

    const match = PROJECT_ROUTE_RE.exec(pathname)
    if (!match) return

    // match[1] is undefined for bare /projects/:id (Overview), otherwise the segment
    const tab = match[1] ?? ''
    if (!TOP_LEVEL_TABS.has(tab)) return

    setLastTabForProject(projectId, tab)
  }, [pathname, projectId, setLastTabForProject])
}
