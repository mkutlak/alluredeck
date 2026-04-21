export interface Labelable {
  slug: string
  display_name?: string
  parent_id?: number | null
  project_id: number
}

function label(project: Labelable): string {
  return project.display_name || project.slug
}

export function formatProjectLabel(
  project: Labelable | undefined,
  allProjects: readonly Labelable[] | undefined,
): string {
  if (!project) return ''
  if (project.parent_id == null) return label(project)
  const parent = allProjects?.find((p) => p.project_id === project.parent_id)
  return parent ? `${label(parent)}/${label(project)}` : label(project)
}
