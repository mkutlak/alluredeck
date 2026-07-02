import { describe, it, expect } from 'vitest'
import { projectNavItems } from './projectNav'
import type { ProjectEntry } from '@/types/api'

function makeProject(overrides: Partial<ProjectEntry> = {}): ProjectEntry {
  return {
    project_id: 1,
    slug: 'my-project',
    display_name: undefined,
    parent_id: null,
    children: [],
    report_type: 'allure',
    ...overrides,
  } as ProjectEntry
}

describe('projectNavItems', () => {
  it('returns all six items for a child/leaf project', () => {
    const project = makeProject({ project_id: 5, children: [] })
    const items = projectNavItems(project)
    expect(items.map((i) => i.label)).toEqual([
      'Overview',
      'Analytics',
      'Defects',
      'Timeline',
      'Known Issues',
      'Attachments',
    ])
  })

  it('returns only the index entry labeled "Pipeline Runs" for a parent project', () => {
    const project = makeProject({ project_id: 5, children: [3, 4] })
    const items = projectNavItems(project)
    expect(items).toHaveLength(1)
    expect(items[0].label).toBe('Pipeline Runs')
  })

  it('the parent index entry links to the bare project route', () => {
    const project = makeProject({ project_id: 5, children: [3, 4] })
    const items = projectNavItems(project)
    expect(items[0].to).toBe('/projects/5')
    expect(items[0].end).toBe(true)
  })

  it('keeps the exact sidebar-nav-* data-testid strings', () => {
    const project = makeProject({ project_id: 5, children: [] })
    const items = projectNavItems(project)
    expect(items.map((i) => i['data-testid'])).toEqual([
      'sidebar-nav-overview',
      'sidebar-nav-analytics',
      'sidebar-nav-defects',
      'sidebar-nav-timeline',
      'sidebar-nav-known-issues',
      'sidebar-nav-attachments',
    ])
  })

  it('builds links scoped to the numeric project id', () => {
    const project = makeProject({ project_id: 42, children: [] })
    const items = projectNavItems(project)
    expect(items.map((i) => i.to)).toEqual([
      '/projects/42',
      '/projects/42/analytics',
      '/projects/42/defects',
      '/projects/42/timeline',
      '/projects/42/known-issues',
      '/projects/42/attachments',
    ])
  })

  it('marks only the Overview item as an exact-match ("end") route', () => {
    const project = makeProject({ project_id: 5, children: [] })
    const items = projectNavItems(project)
    expect(items[0].end).toBe(true)
    expect(items.slice(1).every((i) => !i.end)).toBe(true)
  })

  it('returns an empty array when project is undefined', () => {
    expect(projectNavItems(undefined)).toEqual([])
  })
})
