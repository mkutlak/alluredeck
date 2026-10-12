import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { clearProjectParent, setProjectParent } from '@/api/projects'
import { useProjectDnd, type DndProject } from './useProjectDnd'

vi.mock('@/api/projects', () => ({
  setProjectParent: vi.fn().mockResolvedValue(undefined),
  clearProjectParent: vi.fn().mockResolvedValue(undefined),
}))

// Two groups, each holding a child with the same name: only the ids tell them apart.
const projects: DndProject[] = [
  { label: 'group-a', projectId: 1, parentId: null, hasChildren: true },
  { label: 'group-b', projectId: 2, parentId: null, hasChildren: true },
  { label: 'solo', projectId: 3, parentId: null, hasChildren: false },
  { label: 'e2e', projectId: 5, parentId: 1, hasChildren: false },
  { label: 'e2e', projectId: 9, parentId: 2, hasChildren: false },
]

function renderDnd() {
  const client = new QueryClient()
  return renderHook(() => useProjectDnd(projects), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  })
}

describe('useProjectDnd', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('moves the dragged project by id, not by a slug another project shares', async () => {
    const { result } = renderDnd()

    expect(result.current.moveProject(9, 1)).toEqual({ success: true, targetLabel: 'group-a' })

    await waitFor(() => expect(setProjectParent).toHaveBeenCalledWith(9, 1))
  })

  it('takes a project out of its group by id', async () => {
    const { result } = renderDnd()

    expect(result.current.moveProject(9, null)).toEqual({ success: true })

    await waitFor(() => expect(clearProjectParent).toHaveBeenCalledWith(9))
  })

  it('undoes a move by putting that same project back under its previous group', async () => {
    const { result } = renderDnd()
    result.current.moveProject(9, 1)
    await waitFor(() => expect(setProjectParent).toHaveBeenCalledTimes(1))

    result.current.undoLastMove()

    await waitFor(() => expect(setProjectParent).toHaveBeenLastCalledWith(9, 2))
  })

  it('refuses a drop on itself, on a project with a parent, or of a group', () => {
    const { result } = renderDnd()

    expect(result.current.moveProject(3, 3)).toEqual({ success: false })
    expect(result.current.moveProject(3, 5)).toEqual({ success: false })
    expect(result.current.moveProject(1, 2)).toEqual({ success: false })
    expect(setProjectParent).not.toHaveBeenCalled()
  })
})
