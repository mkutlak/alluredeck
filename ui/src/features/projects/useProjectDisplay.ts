import { useQuery } from '@tanstack/react-query'
import { projectListOptions } from '@/lib/queries'

export function useProjectDisplay(projectId: string | undefined): string {
  const { data } = useQuery(projectListOptions())
  const all = data?.data ?? []
  const project = all.find((p) => p.project_id === Number(projectId))
  return project?.display_name || project?.slug || projectId || ''
}
