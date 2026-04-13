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

function makeBranch(overrides: Partial<Branch> = {}): Branch {
  return {
    id: 1,
    project_id: 1,
    name: 'main',
    is_default: false,
    created_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderSelector(
  props: {
    projectId?: string
    selectedBranch?: string
    onBranchChange?: (branch: string | undefined) => void
  } = {},
) {
  const onBranchChange = props.onBranchChange ?? vi.fn()
  return {
    onBranchChange,
    ...render(
      <QueryClientProvider client={createTestQueryClient()}>
        <BranchSelector
          projectId={props.projectId ?? 'myproject'}
          selectedBranch={props.selectedBranch}
          onBranchChange={onBranchChange}
        />
      </QueryClientProvider>,
    ),
  }
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

  it('shows branch names when branches exist', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'main', is_default: true }),
      makeBranch({ id: 2, name: 'develop', is_default: false }),
    ])
    renderSelector()
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toBeInTheDocument()
    })
  })

  it('shows "All branches" and does not call onBranchChange on mount when selectedBranch is undefined', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'main', is_default: true }),
    ])
    const { onBranchChange } = renderSelector({ selectedBranch: undefined })
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('All branches')
    })
    expect(onBranchChange).not.toHaveBeenCalled()
  })

  it('shows the stored branch when it exists in the branch list', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'master', is_default: true }),
      makeBranch({ id: 2, name: 'dev', is_default: false }),
    ])
    renderSelector({ selectedBranch: 'dev' })
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('dev')
    })
  })

  it('shows "All branches" and does not call onBranchChange when stored branch is absent from list', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'master', is_default: true }),
    ])
    const { onBranchChange } = renderSelector({ selectedBranch: 'dev' })
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('All branches')
    })
    expect(onBranchChange).not.toHaveBeenCalled()
  })

  it('calls onBranchChange with undefined when user selects "All branches"', async () => {
    const user = userEvent.setup()
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'master', is_default: true }),
      makeBranch({ id: 2, name: 'dev', is_default: false }),
    ])
    const { onBranchChange } = renderSelector({ selectedBranch: 'dev' })
    await waitFor(() => screen.getByRole('combobox'))

    await user.click(screen.getByRole('combobox'))
    const allOption = await screen.findByRole('option', { name: 'All branches' })
    await user.click(allOption)

    expect(onBranchChange).toHaveBeenCalledWith(undefined)
  })

  it('calls onBranchChange when a branch is selected', async () => {
    const user = userEvent.setup()
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'main', is_default: true }),
      makeBranch({ id: 2, name: 'develop', is_default: false }),
    ])
    const { onBranchChange } = renderSelector()
    await waitFor(() => screen.getByRole('combobox'))

    await user.click(screen.getByRole('combobox'))
    const developOption = await screen.findByRole('option', { name: 'develop' })
    await user.click(developOption)

    expect(onBranchChange).toHaveBeenCalledWith('develop')
  })

  it('calls onBranchChange with undefined when "All branches" is selected', async () => {
    const user = userEvent.setup()
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'main', is_default: true }),
    ])
    const { onBranchChange } = renderSelector({ selectedBranch: 'main' })
    await waitFor(() => screen.getByRole('combobox'))

    await user.click(screen.getByRole('combobox'))
    const allOption = await screen.findByRole('option', { name: 'All branches' })
    await user.click(allOption)

    expect(onBranchChange).toHaveBeenCalledWith(undefined)
  })

  it('shows a disabled combobox while loading', () => {
    vi.mocked(branchesApi.fetchBranches).mockReturnValue(new Promise(() => {}))
    renderSelector()
    const trigger = screen.getByRole('combobox')
    expect(trigger).toBeDisabled()
  })
})
