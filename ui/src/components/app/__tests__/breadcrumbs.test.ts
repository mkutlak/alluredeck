import { describe, it, expect } from 'vitest'
import { buildBreadcrumbs, LOADING_CRUMB, type BreadcrumbParams } from '../breadcrumbs'
import type { ProjectEntry } from '@/types/api'

const parent: ProjectEntry = {
  project_id: 10,
  slug: 'parent-group',
  display_name: 'Parent Group',
  children: [20],
}
const child: ProjectEntry = {
  project_id: 20,
  slug: 'child-proj',
  display_name: 'Child Project',
  parent_id: 10,
}
const standalone: ProjectEntry = { project_id: 30, slug: 'standalone', display_name: 'Standalone' }
const projects = [parent, child, standalone]

// Crumbs as "label -> href" (with href) or "label" (without).
const trail = (
  pathname: string,
  params: BreadcrumbParams,
  project: ProjectEntry | undefined,
  isLoading = false,
) =>
  buildBreadcrumbs(pathname, params, { project, projects, isLoading })?.map((c) =>
    c.href ? `${c.label} -> ${c.href}` : c.label,
  ) ?? null

describe('buildBreadcrumbs', () => {
  it.each<[string, string, BreadcrumbParams, ProjectEntry | undefined, string[] | null]>([
    ['nothing on the runs feed "/"', '/', {}, undefined, null],
    [
      'root link and plain current project',
      '/projects/30',
      { id: '30' },
      standalone,
      ['Projects -> /', 'Standalone'],
    ],
    [
      'the parent linked by numeric id',
      '/projects/20',
      { id: '20' },
      child,
      ['Projects -> /', 'Parent Group -> /projects/10', 'Child Project'],
    ],
    [
      'the current tab as plain text',
      '/projects/30/analytics',
      { id: '30' },
      standalone,
      ['Projects -> /', 'Standalone', 'Analytics'],
    ],
    [
      'a linked Overview tab and plain report under reports/',
      '/projects/30/reports/42',
      { id: '30', reportId: '42' },
      standalone,
      ['Projects -> /', 'Standalone', 'Overview -> /projects/30', 'Report #42'],
    ],
    [
      'a linked Attachments tab (numeric id for a slug param) and plain source under trace/',
      '/projects/standalone/trace/my-trace.zip',
      { id: 'standalone', source: 'my-trace.zip' },
      standalone,
      ['Projects -> /', 'Standalone', 'Attachments -> /projects/30/attachments', 'my-trace.zip'],
    ],
    [
      '"Build Comparison" on compare',
      '/projects/30/compare',
      { id: '30' },
      standalone,
      ['Projects -> /', 'Standalone', 'Build Comparison'],
    ],
    // A slug route param must still link by the numeric project_id.
    [
      'numeric project_id links for a slug param',
      '/projects/child-proj/reports/99',
      { id: 'child-proj', reportId: '99' },
      child,
      [
        'Projects -> /',
        'Parent Group -> /projects/10',
        'Child Project',
        'Overview -> /projects/20',
        'Report #99',
      ],
    ],
  ])('builds %s', (_, pathname, params, project, expected) => {
    expect(trail(pathname, params, project)).toEqual(expected)
  })

  it('returns the loading sentinel while the project resolves', () => {
    expect(trail('/projects/30', { id: '30' }, undefined, true)).toEqual([LOADING_CRUMB])
  })
})
