import { useQuery } from '@tanstack/react-query'
import { getProject } from '@/api/projects'
import { projectIndexOptions } from '@/lib/queries/projects'
import type { ProjectEntry } from '@/types/api'

export function resolveProjectFromParam(
  param: string | undefined,
  projects: readonly ProjectEntry[] | undefined,
): ProjectEntry | undefined {
  if (!param || !projects) return undefined

  if (/^\d+$/.test(param)) {
    return projects.find((p) => p.project_id === Number(param))
  }

  return projects.find((p) => p.slug === param)
}

export function useProjectFromParam(param: string | undefined): {
  project: ProjectEntry | undefined
  projects: readonly ProjectEntry[] | undefined
  isLoading: boolean
  error: unknown
} {
  const { data, isLoading: listLoading, error: listError } = useQuery(projectIndexOptions())
  const projects = data?.data
  const projectFromList = resolveProjectFromParam(param, projects)

  const shouldFetchSingle = !listLoading && !projectFromList && !!param
  const {
    data: singleData,
    isPending: singlePending,
    error: singleError,
  } = useQuery({
    queryKey: ['project', param],
    queryFn: () => getProject(param!),
    enabled: shouldFetchSingle,
    staleTime: 5_000,
  })

  const project = projectFromList ?? singleData?.data
  const isLoading = listLoading || (shouldFetchSingle && singlePending)
  const error = projectFromList ? null : (singleError ?? listError)

  return { project, projects, isLoading, error }
}
