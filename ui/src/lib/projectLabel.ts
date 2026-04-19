import type { ProjectEntry } from '@/types/api'

export function formatProjectLabel(
  project: ProjectEntry | undefined,
  allProjects: readonly ProjectEntry[] | undefined,
): string {
  if (!project) return ''
  if (project.parent_id == null) return project.slug
  const parent = allProjects?.find((p) => p.project_id === project.parent_id)
  return parent ? `${parent.slug}/${project.slug}` : project.slug
}
