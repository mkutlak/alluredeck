import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useActiveProject } from './useActiveProject'
import type { UIState } from '@/store/ui'

interface ActiveProjectResult {
  projectId: string | null
  isFromUrl: boolean
  isLoading: boolean
}

const mockSetLastProjectId = vi.fn()

vi.mock('@/api/projects', () => ({
  getProjects: vi.fn(),
}))

vi.mock('@/store/ui', () => ({
  useUIStore: vi.fn((selector: (s: UIState) => unknown) =>
    selector({
      lastProjectId: null,
      setLastProjectId: mockSetLastProjectId,
      clearLastProjectId: vi.fn(),
      projectViewMode: 'grid',
      setProjectViewMode: vi.fn(),
    })
  ),
}))

import { getProjects } from '@/api/projects'
import { useUIStore } from '@/store/ui'

const mockGetProjects = vi.mocked(getProjects)
const mockUseUIStore = vi.mocked(useUIStore)

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
}

function TestHook({ onResult }: { onResult: (r: ActiveProjectResult) => void }) {
  const result = useActiveProject()
  onResult(result)
  return null
}

function renderHookWithUrl(
  path: string,
  routePattern: string,
  onResult: (r: ActiveProjectResult) => void,
  queryClient: QueryClient
) {
  return render(
    createElement(QueryClientProvider, { client: queryClient },
      createElement(MemoryRouter, { initialEntries: [path] },
        createElement(Routes, null,
          createElement(Route, { path: routePattern, element: createElement(TestHook, { onResult }) })
        )
      )
    )
  )
}

function renderHookNoUrl(
  onResult: (r: ActiveProjectResult) => void,
  queryClient: QueryClient
) {
  return render(
    createElement(QueryClientProvider, { client: queryClient },
      createElement(MemoryRouter, { initialEntries: ['/'] },
        createElement(Routes, null,
          createElement(Route, { path: '/', element: createElement(TestHook, { onResult }) })
        )
      )
    )
  )
}

describe('useActiveProject', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetProjects.mockResolvedValue({
      data: [{ project_id: 'first-project' }, { project_id: 'second-project' }],
      metadata: { message: 'ok' },
      pagination: { total: 2, page: 1, per_page: 20, total_pages: 1 },
    })
    mockUseUIStore.mockImplementation((selector: (s: UIState) => unknown) =>
      selector({
        lastProjectId: null,
        setLastProjectId: mockSetLastProjectId,
        clearLastProjectId: vi.fn(),
        projectViewMode: 'grid',
        setProjectViewMode: vi.fn(),
      })
    )
  })

  it('returns urlProjectId when URL has /projects/:id, isFromUrl true, isLoading false', async () => {
    const results: ActiveProjectResult[] = []
    const queryClient = makeQueryClient()

    renderHookWithUrl(
      '/projects/my-project',
      '/projects/:id',
      (r) => results.push(r),
      queryClient
    )

    await waitFor(() => {
      expect(results.length).toBeGreaterThan(0)
    })

    const last = results[results.length - 1]
    expect(last.projectId).toBe('my-project')
    expect(last.isFromUrl).toBe(true)
    expect(last.isLoading).toBe(false)
  })

  it('returns stored lastProjectId when no URL param and store has value, isFromUrl false', async () => {
    mockUseUIStore.mockImplementation((selector: (s: UIState) => unknown) =>
      selector({
        lastProjectId: 'stored-project',
        setLastProjectId: mockSetLastProjectId,
        clearLastProjectId: vi.fn(),
        projectViewMode: 'grid',
        setProjectViewMode: vi.fn(),
      })
    )

    const results: ActiveProjectResult[] = []
    const queryClient = makeQueryClient()

    renderHookNoUrl((r) => results.push(r), queryClient)

    await waitFor(() => {
      expect(results.length).toBeGreaterThan(0)
    })

    const last = results[results.length - 1]
    expect(last.projectId).toBe('stored-project')
    expect(last.isFromUrl).toBe(false)
    expect(last.isLoading).toBe(false)
  })

  it('auto-selects first project from API when no URL param and no stored project', async () => {
    const results: ActiveProjectResult[] = []
    const queryClient = makeQueryClient()

    renderHookNoUrl((r) => results.push(r), queryClient)

    await waitFor(() => {
      const last = results[results.length - 1]
      expect(last.projectId).toBe('first-project')
    })

    const last = results[results.length - 1]
    expect(last.isFromUrl).toBe(false)
  })

  it('returns null when no URL param, no stored project, and empty project list', async () => {
    mockGetProjects.mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
      pagination: { total: 0, page: 1, per_page: 20, total_pages: 0 },
    })

    const results: ActiveProjectResult[] = []
    const queryClient = makeQueryClient()

    renderHookNoUrl((r) => results.push(r), queryClient)

    await waitFor(() => {
      const last = results[results.length - 1]
      expect(last.isLoading).toBe(false)
    })

    const last = results[results.length - 1]
    expect(last.projectId).toBeNull()
    expect(last.isFromUrl).toBe(false)
  })

  it('calls setLastProjectId when URL urlProjectId is truthy', async () => {
    const results: ActiveProjectResult[] = []
    const queryClient = makeQueryClient()

    renderHookWithUrl(
      '/projects/url-project',
      '/projects/:id',
      (r) => results.push(r),
      queryClient
    )

    await waitFor(() => {
      expect(results.length).toBeGreaterThan(0)
    })

    expect(mockSetLastProjectId).toHaveBeenCalledWith('url-project')
  })
})
