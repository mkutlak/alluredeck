import type { ProjectEntry } from '@/types/api'

export interface ProjectGroup<T> {
  parentId: number | null
  parentLabel: string
  items: T[]
}

export function groupByParent<T extends { project_id: number }>(
  entries: T[],
  projects: readonly ProjectEntry[] | undefined,
): ProjectGroup<T>[] {
  if (!projects || projects.length === 0) {
    return entries.map((entry) => ({
      parentId: null,
      parentLabel: '',
      items: [entry],
    }))
  }

  const projectMap = new Map(projects.map((p) => [p.project_id, p]))
  const groups = new Map<number, ProjectGroup<T>>()
  const standalone: ProjectGroup<T>[] = []

  for (const entry of entries) {
    const proj = projectMap.get(entry.project_id)
    if (proj?.parent_id != null) {
      const existing = groups.get(proj.parent_id)
      if (existing) {
        existing.items.push(entry)
      } else {
        const parent = projectMap.get(proj.parent_id)
        groups.set(proj.parent_id, {
          parentId: proj.parent_id,
          parentLabel: parent?.display_name || parent?.slug || `#${proj.parent_id}`,
          items: [entry],
        })
      }
    } else {
      standalone.push({ parentId: null, parentLabel: '', items: [entry] })
    }
  }

  const sorted = Array.from(groups.values()).sort((a, b) =>
    a.parentLabel.localeCompare(b.parentLabel),
  )
  return [...sorted, ...standalone]
}
