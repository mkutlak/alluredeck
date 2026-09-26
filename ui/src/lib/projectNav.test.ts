import { describe, it, expect } from 'vitest'
import { projectNavItems } from './projectNav'
import type { ProjectEntry } from '@/types/api'

const project = (overrides: Partial<ProjectEntry>): ProjectEntry => ({
  project_id: 42,
  slug: 'my-project',
  children: [],
  ...overrides,
})

describe('projectNavItems', () => {
  it('gives a leaf project every tab, linked by numeric project id', () => {
    expect(projectNavItems(project({})).map((i) => i.to)).toEqual([
      '/projects/42',
      '/projects/42/analytics',
      '/projects/42/defects',
      '/projects/42/timeline',
      '/projects/42/known-issues',
      '/projects/42/attachments',
    ])
  })

  it('gives a parent project only its index entry, relabeled "Pipeline Runs"', () => {
    expect(projectNavItems(project({ project_id: 5, children: [3, 4] }))).toEqual([
      {
        to: '/projects/5',
        label: 'Pipeline Runs',
        end: true,
        'data-testid': 'sidebar-nav-overview',
      },
    ])
  })

  it('returns an empty array when project is undefined', () => {
    expect(projectNavItems(undefined)).toEqual([])
  })
})
