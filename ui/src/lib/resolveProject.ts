import { useQuery } from '@tanstack/react-query'
import { projectListOptions } from '@/lib/queries/projects'
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
  isLoading: boolean
  error: unknown
} {
  const { data, isLoading, error } = useQuery(projectListOptions())
  const projects = data?.data
  const project = resolveProjectFromParam(param, projects)
  return { project, isLoading, error }
}
