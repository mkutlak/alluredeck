import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { resolveProjectFromParam, useProjectFromParam } from './resolveProject'
import type { ProjectEntry } from '@/types/api'

// ---------------------------------------------------------------------------
// resolveProjectFromParam — pure function tests
// ---------------------------------------------------------------------------

const projects: ProjectEntry[] = [
  { project_id: 1, slug: 'alpha' },
  { project_id: 42, slug: 'beta' },
  { project_id: 100, slug: '99problems' },
]

describe('resolveProjectFromParam', () => {
  it('returns undefined when param is undefined', () => {
    expect(resolveProjectFromParam(undefined, projects)).toBeUndefined()
  })

  it('returns undefined when param is empty string', () => {
    expect(resolveProjectFromParam('', projects)).toBeUndefined()
  })

  it('returns undefined when projects is undefined', () => {
    expect(resolveProjectFromParam('alpha', undefined)).toBeUndefined()
  })

  it('returns undefined for empty projects array', () => {
    expect(resolveProjectFromParam('alpha', [])).toBeUndefined()
  })

  it('matches by project_id when param is a pure-digit string', () => {
    const result = resolveProjectFromParam('42', projects)
    expect(result).toBe(projects[1])
  })

  it('matches by slug when param is not a pure-digit string', () => {
    const result = resolveProjectFromParam('alpha', projects)
    expect(result).toBe(projects[0])
  })

  it('"42abc" falls through to slug match, not numeric match', () => {
    // '42abc' is not /^\d+$/ so it tries slug match; no slug '42abc' exists
    expect(resolveProjectFromParam('42abc', projects)).toBeUndefined()
  })

  it('slug that starts with digits but has letters is matched as slug', () => {
    // '99problems' slug exists
    const result = resolveProjectFromParam('99problems', projects)
    expect(result).toBe(projects[2])
  })

  it('returns the exact project object, not a copy', () => {
    const result = resolveProjectFromParam('1', projects)
    expect(result).toBe(projects[0])
  })
})

// ---------------------------------------------------------------------------
// useProjectFromParam — hook tests
// ---------------------------------------------------------------------------

vi.mock('@/api/projects', () => ({
  getProjects: vi.fn(),
  getProjectIndex: vi.fn(),
}))

import { getProjectIndex, getProjects } from '@/api/projects'

const mockProject: ProjectEntry = { project_id: 7, slug: 'myproject' }

function makeWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
}

describe('useProjectFromParam', () => {
  beforeEach(() => {
    vi.mocked(getProjectIndex).mockResolvedValue({
      data: [mockProject],
      metadata: { message: 'ok' },
    })
    vi.mocked(getProjects).mockResolvedValue({
      data: [mockProject],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 50, total: 1, total_pages: 1 },
    })
  })

  it('resolves project by slug after data loads', async () => {
    const { result } = renderHook(() => useProjectFromParam('myproject'), {
      wrapper: makeWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.project).toBe(result.current.project)
    expect(result.current.project?.project_id).toBe(7)
    expect(result.current.error).toBeNull()
  })

  it('resolves project by numeric id after data loads', async () => {
    const { result } = renderHook(() => useProjectFromParam('7'), {
      wrapper: makeWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.project?.slug).toBe('myproject')
  })

  it('returns undefined project when param does not match', async () => {
    const { result } = renderHook(() => useProjectFromParam('nonexistent'), {
      wrapper: makeWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.project).toBeUndefined()
  })

  it('returns undefined project when param is undefined', async () => {
    const { result } = renderHook(() => useProjectFromParam(undefined), {
      wrapper: makeWrapper(),
    })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.project).toBeUndefined()
  })
})
