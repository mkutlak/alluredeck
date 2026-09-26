import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { createTestQueryClient } from '@/test/render'
import { resolveProjectFromParam, useProjectFromParam } from './resolveProject'
import type { ProjectEntry } from '@/types/api'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn(),
  getProject: vi.fn(),
}))

import { getProject, getProjectIndex } from '@/api/projects'

const projects: ProjectEntry[] = [
  { project_id: 1, slug: 'alpha' },
  { project_id: 42, slug: 'beta' },
  { project_id: 100, slug: '99problems' },
]

describe('resolveProjectFromParam', () => {
  // Pure-digit params match project_id, anything else matches slug; the list's
  // own object is returned, not a copy.
  it.each([
    ['42', 'the list', projects, projects[1]],
    ['1', 'the list', projects, projects[0]],
    ['alpha', 'the list', projects, projects[0]],
    ['99problems', 'the list', projects, projects[2]],
    ['42abc', 'the list', projects, undefined],
    [undefined, 'the list', projects, undefined],
    ['', 'the list', projects, undefined],
    ['alpha', 'no list', undefined, undefined],
    ['alpha', 'an empty list', [], undefined],
  ])('resolves %j against %s', (param, _, list, expected) => {
    expect(resolveProjectFromParam(param, list)).toBe(expected)
  })
})

describe('useProjectFromParam', () => {
  const notFound = new Error('not found')

  beforeEach(() => {
    vi.mocked(getProjectIndex).mockResolvedValue({
      data: [{ project_id: 7, slug: 'myproject' }],
      metadata: { message: 'ok' },
    })
    // Params missing from the index fall back to a single-project fetch.
    vi.mocked(getProject).mockRejectedValue(notFound)
  })

  it.each([
    ['7', 7, null],
    ['myproject', 7, null],
    ['nonexistent', undefined, notFound],
    [undefined, undefined, null],
  ])('resolves %j to project_id %s (error %s)', async (param, expectedId, expectedError) => {
    const qc = createTestQueryClient()
    const { result } = renderHook(() => useProjectFromParam(param), {
      wrapper: ({ children }) => createElement(QueryClientProvider, { client: qc }, children),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.project?.project_id).toBe(expectedId)
    expect(result.current.error).toBe(expectedError)
  })
})
