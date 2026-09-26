import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { BranchSelector } from '../BranchSelector'
import * as branchesApi from '@/api/branches'
import type { Branch } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/branches')
mockApiClient()

function branches(...names: string[]): Branch[] {
  return names.map((name, i) => ({
    id: i + 1,
    project_id: 1,
    name,
    is_default: i === 0,
    created_at: '2024-01-01T00:00:00Z',
  }))
}

function renderSelector(selectedBranch?: string) {
  const onBranchChange = vi.fn()
  const view = render(
    <QueryClientProvider client={createTestQueryClient()}>
      <BranchSelector
        projectId="myproject"
        selectedBranch={selectedBranch}
        onBranchChange={onBranchChange}
      />
    </QueryClientProvider>,
  )
  return { onBranchChange, ...view }
}

describe('BranchSelector', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when branches list is empty', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([])
    const { container } = renderSelector()
    await waitFor(() => {
      expect(container.firstChild).toBeNull()
    })
  })

  it('shows a disabled combobox while loading', () => {
    vi.mocked(branchesApi.fetchBranches).mockReturnValue(new Promise(() => {}))
    renderSelector()
    expect(screen.getByRole('combobox')).toBeDisabled()
  })

  // Regression (d66811e): undefined means "All branches", and the selector never
  // writes a branch back on its own; a stored branch this project lacks shows
  // "All branches" without clearing the remembered choice.
  it.each<[string, string | undefined, string]>([
    [
      'shows "All branches" and does not call onBranchChange on mount when selectedBranch is undefined',
      undefined,
      'All branches',
    ],
    ['shows the stored branch when it exists in the branch list', 'dev', 'dev'],
    [
      'shows "All branches" and does not call onBranchChange when stored branch is absent from list',
      'gone',
      'All branches',
    ],
  ])('%s', async (_name, stored, shown) => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue(branches('master', 'dev'))
    const { onBranchChange } = renderSelector(stored)

    // The loading placeholder also reads "All branches"; wait for the real list.
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled())
    expect(screen.getByRole('combobox')).toHaveTextContent(shown)
    expect(onBranchChange).not.toHaveBeenCalled()
  })

  it.each<[string, string | undefined, string, string | undefined]>([
    ['calls onBranchChange when a branch is selected', undefined, 'dev', 'dev'],
    [
      'calls onBranchChange with undefined when user selects "All branches"',
      'dev',
      'All branches',
      undefined,
    ],
  ])('%s', async (_name, stored, option, want) => {
    const user = userEvent.setup()
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue(branches('master', 'dev'))
    const { onBranchChange } = renderSelector(stored)
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled())

    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: option }))

    expect(onBranchChange).toHaveBeenCalledWith(want)
  })
})
