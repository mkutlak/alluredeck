import { describe, it, expect } from 'vitest'
import { formatProjectLabel } from '@/lib/projectLabel'
import type { ProjectEntry } from '@/types/api'

const PARENT_A: ProjectEntry = { project_id: 1, slug: 'parent-a' }
const PARENT_B: ProjectEntry = { project_id: 2, slug: 'parent-b' }
const CHILD_A_OF_PARENT_A: ProjectEntry = { project_id: 10, slug: 'child-a', parent_id: 1 }
const CHILD_A_OF_PARENT_B: ProjectEntry = { project_id: 11, slug: 'child-a', parent_id: 2 }
const STANDALONE: ProjectEntry = { project_id: 20, slug: 'standalone' }
const NAMED_PARENT: ProjectEntry = { ...PARENT_A, display_name: 'Parent A' }
const NAMED_CHILD: ProjectEntry = { ...CHILD_A_OF_PARENT_A, display_name: 'Child A' }

describe('formatProjectLabel', () => {
  it.each([
    ['an undefined project', undefined, [], ''],
    ['a top-level project', STANDALONE, [STANDALONE], 'standalone'],
    ['a top-level project without a list', STANDALONE, undefined, 'standalone'],
    ['a nested project', CHILD_A_OF_PARENT_A, [PARENT_A, CHILD_A_OF_PARENT_A], 'parent-a/child-a'],
    [
      'a nested project whose parent is not listed',
      CHILD_A_OF_PARENT_A,
      [CHILD_A_OF_PARENT_A],
      'child-a',
    ],
    ['a nested project without a list', CHILD_A_OF_PARENT_A, undefined, 'child-a'],
    [
      'display_name on a top-level project',
      { ...STANDALONE, display_name: 'My Project' },
      [],
      'My Project',
    ],
    [
      'display_name on parent and child',
      NAMED_CHILD,
      [NAMED_PARENT, NAMED_CHILD],
      'Parent A/Child A',
    ],
    ['display_name on the child only', NAMED_CHILD, [PARENT_A, NAMED_CHILD], 'parent-a/Child A'],
    ['display_name on the parent only', CHILD_A_OF_PARENT_A, [NAMED_PARENT], 'Parent A/child-a'],
    ['an empty display_name', { ...STANDALONE, display_name: '' }, [], 'standalone'],
  ])('labels %s', (_, project, all, expected) => {
    expect(formatProjectLabel(project, all)).toBe(expected)
  })

  it('disambiguates same-named children under different parents', () => {
    const all = [PARENT_A, PARENT_B, CHILD_A_OF_PARENT_A, CHILD_A_OF_PARENT_B]
    expect(formatProjectLabel(CHILD_A_OF_PARENT_A, all)).toBe('parent-a/child-a')
    expect(formatProjectLabel(CHILD_A_OF_PARENT_B, all)).toBe('parent-b/child-a')
  })
})
