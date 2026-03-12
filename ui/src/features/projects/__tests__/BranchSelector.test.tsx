import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BranchSelector } from '../BranchSelector'
import * as branchesApi from '@/api/branches'
import type { Branch } from '@/types/api'

vi.mock('@/api/branches')
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function makeBranch(overrides: Partial<Branch> = {}): Branch {
  return {
    id: 1,
    project_id: 'myproject',
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
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    onBranchChange,
    ...render(
      <QueryClientProvider client={qc}>
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

  it('pre-selects the default branch', async () => {
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      makeBranch({ id: 1, name: 'main', is_default: false }),
      makeBranch({ id: 2, name: 'release', is_default: true }),
    ])
    renderSelector()
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('release')
    })
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
