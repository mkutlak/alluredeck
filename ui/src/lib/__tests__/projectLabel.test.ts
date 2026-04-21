import { describe, it, expect } from 'vitest'
import { formatProjectLabel } from '@/lib/projectLabel'
import type { ProjectEntry } from '@/types/api'

const PARENT_A: ProjectEntry = { project_id: 1, slug: 'parent-a' }
const PARENT_B: ProjectEntry = { project_id: 2, slug: 'parent-b' }
const CHILD_A_OF_PARENT_A: ProjectEntry = { project_id: 10, slug: 'child-a', parent_id: 1 }
const CHILD_A_OF_PARENT_B: ProjectEntry = { project_id: 11, slug: 'child-a', parent_id: 2 }
const STANDALONE: ProjectEntry = { project_id: 20, slug: 'standalone' }

describe('formatProjectLabel', () => {
  it('returns empty string when project is undefined', () => {
    expect(formatProjectLabel(undefined, [])).toBe('')
  })

  it('returns slug for top-level project', () => {
    expect(formatProjectLabel(STANDALONE, [STANDALONE])).toBe('standalone')
  })

  it('returns parent/child for nested project', () => {
    const all = [PARENT_A, CHILD_A_OF_PARENT_A]
    expect(formatProjectLabel(CHILD_A_OF_PARENT_A, all)).toBe('parent-a/child-a')
  })

  it('disambiguates same-named children under different parents', () => {
    const all = [PARENT_A, PARENT_B, CHILD_A_OF_PARENT_A, CHILD_A_OF_PARENT_B]
    expect(formatProjectLabel(CHILD_A_OF_PARENT_A, all)).toBe('parent-a/child-a')
    expect(formatProjectLabel(CHILD_A_OF_PARENT_B, all)).toBe('parent-b/child-a')
  })

  it('falls back to slug when parent not found in list', () => {
    expect(formatProjectLabel(CHILD_A_OF_PARENT_A, [CHILD_A_OF_PARENT_A])).toBe('child-a')
  })

  it('handles undefined allProjects', () => {
    expect(formatProjectLabel(STANDALONE, undefined)).toBe('standalone')
    expect(formatProjectLabel(CHILD_A_OF_PARENT_A, undefined)).toBe('child-a')
  })

  it('prefers display_name over slug for top-level project', () => {
    const p: ProjectEntry = { project_id: 30, slug: 'proj-30', display_name: 'My Project' }
    expect(formatProjectLabel(p, [p])).toBe('My Project')
  })

  it('prefers display_name over slug for child project', () => {
    const parent: ProjectEntry = { project_id: 1, slug: 'parent-a', display_name: 'Parent A' }
    const child: ProjectEntry = { project_id: 10, slug: 'child-a', display_name: 'Child A', parent_id: 1 }
    expect(formatProjectLabel(child, [parent, child])).toBe('Parent A/Child A')
  })

  it('uses display_name for child only when parent has no display_name', () => {
    const parent: ProjectEntry = { project_id: 1, slug: 'parent-a' }
    const child: ProjectEntry = { project_id: 10, slug: 'child-a', display_name: 'Child A', parent_id: 1 }
    expect(formatProjectLabel(child, [parent, child])).toBe('parent-a/Child A')
  })

  it('uses display_name for parent only when child has no display_name', () => {
    const parent: ProjectEntry = { project_id: 1, slug: 'parent-a', display_name: 'Parent A' }
    const child: ProjectEntry = { project_id: 10, slug: 'child-a', parent_id: 1 }
    expect(formatProjectLabel(child, [parent, child])).toBe('Parent A/child-a')
  })

  it('falls back to slug when display_name is empty string', () => {
    const p: ProjectEntry = { project_id: 30, slug: 'proj-30', display_name: '' }
    expect(formatProjectLabel(p, [p])).toBe('proj-30')
  })
})
