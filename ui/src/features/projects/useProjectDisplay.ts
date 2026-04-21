import { useQuery } from '@tanstack/react-query'
import { projectIndexOptions } from '@/lib/queries'
import { resolveProjectFromParam } from '@/lib/resolveProject'
import { formatProjectLabel } from '@/lib/projectLabel'

export function useProjectDisplay(projectId: string | undefined): string {
  const { data } = useQuery(projectIndexOptions())
  const project = resolveProjectFromParam(projectId, data?.data)
  if (project) return formatProjectLabel(project, data?.data)
  // While the projects list is loading or the id is unknown:
  // - slug-style params are safe to render verbatim (they are slugs, not IDs)
  // - numeric params are project_ids; never leak them into the UI
  if (projectId && !/^\d+$/.test(projectId)) return projectId
  if (projectId) return '…'
  return ''
}
