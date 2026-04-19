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
})
