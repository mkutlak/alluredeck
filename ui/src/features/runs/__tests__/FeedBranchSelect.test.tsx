import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { FeedBranchSelect } from '../FeedBranchSelect'
import { useUIStore } from '@/store/ui'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 10, slug: 'acme', parent_id: null, children: [1, 2] },
      { project_id: 20, slug: 'globex', parent_id: null, children: [3] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn(),
}))

import { fetchBranches } from '@/api/branches'

function makeBranch(name: string, projectId: number) {
  return { id: 1, project_id: projectId, name, is_default: false, created_at: '2024-01-01T00:00:00Z' }
}

describe('FeedBranchSelect', () => {
  beforeEach(() => {
    vi.mocked(fetchBranches).mockReset()
    useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
  })

  it('unions branch names across all parent groups when none is selected', async () => {
    vi.mocked(fetchBranches).mockImplementation((projectId: string) =>
      Promise.resolve(
        projectId === '10' ? [makeBranch('main', 10)] : [makeBranch('develop', 20)],
      ),
    )
    const user = userEvent.setup()
    renderWithProviders(<FeedBranchSelect />)

    await waitFor(() => expect(screen.getByRole('combobox')).not.toBeDisabled())
    await user.click(screen.getByRole('combobox'))

    expect(await screen.findByRole('option', { name: 'main' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'develop' })).toBeInTheDocument()
  })

  it('only queries selected groups when runsFeedGroupIds is non-empty', async () => {
    useUIStore.setState({ runsFeedGroupIds: [10] })
    vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main', 10)])
    renderWithProviders(<FeedBranchSelect />)

    await waitFor(() => {
      expect(fetchBranches).toHaveBeenCalledWith('10')
    })
    expect(fetchBranches).not.toHaveBeenCalledWith('20')
  })

  it('shows "All branches" when the stored selectedBranch is not in the union', async () => {
    useUIStore.setState({ selectedBranch: 'nonexistent' })
    vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main', 10)])
    renderWithProviders(<FeedBranchSelect />)

    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('All branches')
    })
  })

  it('shows the stored branch when it is present in the union', async () => {
    useUIStore.setState({ selectedBranch: 'main' })
    vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main', 10)])
    renderWithProviders(<FeedBranchSelect />)

    await waitFor(() => {
      expect(screen.getByRole('combobox')).toHaveTextContent('main')
    })
  })

  it('calls setSelectedBranch when a branch is chosen', async () => {
    vi.mocked(fetchBranches).mockResolvedValue([makeBranch('main', 10), makeBranch('develop', 20)])
    const user = userEvent.setup()
    renderWithProviders(<FeedBranchSelect />)

    await waitFor(() => expect(screen.getByRole('combobox')).not.toBeDisabled())
    await user.click(screen.getByRole('combobox'))
    const option = await screen.findByRole('option', { name: 'develop' })
    await user.click(option)

    expect(useUIStore.getState().selectedBranch).toBe('develop')
  })
})
