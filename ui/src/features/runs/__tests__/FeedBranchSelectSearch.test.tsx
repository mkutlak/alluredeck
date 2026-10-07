import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { FeedBranchSelect } from '../FeedBranchSelect'
import { useUIStore } from '@/store/ui'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', parent_id: null, children: [1, 2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn(),
}))

import { fetchBranches } from '@/api/branches'

const branch = (name: string) => ({
  id: 1,
  project_id: 10,
  name,
  is_default: false,
  created_at: '2024-01-01T00:00:00Z',
})

// The feed's branch lives in the URL; the router is returned so a test can read it.
async function openBranches(url = '/') {
  const user = userEvent.setup()
  const router = createMemoryRouter([{ path: '/', element: <FeedBranchSelect /> }], {
    initialEntries: [url],
  })
  renderWithProviders(<></>, { router })
  const trigger = await screen.findByRole('combobox', { name: /filter by branch/i })
  await waitFor(() => expect(trigger).toBeEnabled())
  await user.click(trigger)
  return { user, router }
}

describe('FeedBranchSelect search and clear', () => {
  beforeEach(() => {
    vi.mocked(fetchBranches).mockReset()
    vi.mocked(fetchBranches).mockResolvedValue([
      branch('main'),
      branch('develop'),
      branch('feature/login'),
      branch('feature/checkout'),
    ])
    useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
  })

  it('filters the branch list as you type', async () => {
    const { user } = await openBranches()
    expect(await screen.findAllByRole('option')).toHaveLength(4)

    await user.type(screen.getByPlaceholderText(/search branches/i), 'feature')

    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual([
      'feature/checkout',
      'feature/login',
    ])
  })

  it('selects the typed-for branch', async () => {
    const { user, router } = await openBranches()
    await user.type(await screen.findByPlaceholderText(/search branches/i), 'login')
    await user.click(screen.getByRole('option', { name: 'feature/login' }))

    expect(new URLSearchParams(router.state.location.search).get('branch')).toBe('feature/login')
  })

  // The old Select let you pick "All branches"; a clear inside the same filter
  // replaces it without adding a control.
  it('clears the selected branch from inside the filter', async () => {
    const { user, router } = await openBranches('/?branch=main')
    await user.click(await screen.findByText(/clear/i))

    expect(new URLSearchParams(router.state.location.search).has('branch')).toBe(false)
  })

  it('offers no clear while no branch is selected', async () => {
    await openBranches()
    await screen.findAllByRole('option')
    expect(screen.queryByText(/clear/i)).not.toBeInTheDocument()
  })
})
