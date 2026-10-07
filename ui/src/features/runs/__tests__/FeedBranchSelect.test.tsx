import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
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
  return {
    id: 1,
    project_id: projectId,
    name,
    is_default: false,
    created_at: '2024-01-01T00:00:00Z',
  }
}

async function openOptions() {
  const user = userEvent.setup()
  renderWithProviders(<FeedBranchSelect />)
  await waitFor(() => expect(screen.getByRole('combobox')).not.toBeDisabled())
  await user.click(screen.getByRole('combobox'))
  return user
}

describe('FeedBranchSelect', () => {
  beforeEach(() => {
    vi.mocked(fetchBranches).mockReset()
    vi.mocked(fetchBranches).mockImplementation((projectId: string) =>
      Promise.resolve(projectId === '10' ? [makeBranch('main', 10)] : [makeBranch('develop', 20)]),
    )
    useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
  })

  it.each([
    { name: 'all parent groups', groups: [], absent: [] },
    { name: 'only the selected groups', groups: [10], absent: ['develop'] },
  ])('offers the union of branches across $name', async ({ groups, absent }) => {
    useUIStore.setState({ runsFeedGroupIds: groups })
    await openOptions()

    expect(await screen.findByRole('option', { name: 'main' })).toBeInTheDocument()
    for (const name of ['develop'].filter((n) => !absent.includes(n))) {
      expect(screen.getByRole('option', { name })).toBeInTheDocument()
    }
    for (const name of absent) {
      expect(screen.queryByRole('option', { name })).not.toBeInTheDocument()
    }
  })

  // The URL carries the feed's branch; the global one belongs to the project pages.
  // Assert after loading: the disabled loading state always reads "All branches".
  it.each([
    { route: '/?branch=main', stored: undefined, shown: 'main' },
    { route: '/', stored: 'main', shown: 'All branches' },
  ])('shows $shown at $route with stored branch $stored', async ({ route, stored, shown }) => {
    useUIStore.setState({ selectedBranch: stored })
    renderWithProviders(<FeedBranchSelect />, { route })

    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled())
    expect(screen.getByRole('combobox')).toHaveTextContent(shown)
  })

  it('writes the chosen branch to the URL, not to the global store', async () => {
    const user = userEvent.setup()
    const router = createMemoryRouter([{ path: '/', element: <FeedBranchSelect /> }])
    renderWithProviders(<></>, { router })
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled())
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: 'develop' }))

    expect(router.state.location.search).toBe('?branch=develop')
    expect(useUIStore.getState().selectedBranch).toBeUndefined()
  })
})
