import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunsFeedPage } from '../RunsFeedPage'
import { useUIStore } from '@/store/ui'
import type { PaginatedResponse, PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunsFeed: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunFailures: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 10, slug: 'acme', display_name: 'Acme', parent_id: null, children: [1] },
      { project_id: 20, slug: 'globex', parent_id: null, children: [2] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/branches', () => ({
  fetchBranches: vi.fn().mockResolvedValue([
    { id: 1, project_id: 10, name: 'main', is_default: true, created_at: '2024-01-01T00:00:00Z' },
    {
      id: 2,
      project_id: 10,
      name: 'develop',
      is_default: false,
      created_at: '2024-01-01T00:00:00Z',
    },
  ]),
}))

import { fetchRunsFeed } from '@/api/pipeline'

function feed(totalPages = 3): PaginatedResponse<PipelineRun[]> {
  const run: PipelineRun = {
    pipeline_id: '42',
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    suites: [],
    aggregate: {
      suites_passed: 0,
      suites_total: 0,
      tests_passed: 0,
      tests_total: 0,
      pass_rate: 0,
      total_duration_ms: 0,
    },
  }
  return {
    data: [run],
    metadata: { message: 'ok' },
    pagination: { page: 1, per_page: 10, total: 1, total_pages: totalPages },
  }
}

// The router is returned so a test can read the URL the feed wrote.
function renderFeedAt(url: string) {
  const router = createMemoryRouter([{ path: '/', element: <RunsFeedPage /> }], {
    initialEntries: [url],
  })
  renderWithProviders(<></>, { router })
  return router
}

const lastFeedCall = () => vi.mocked(fetchRunsFeed).mock.lastCall

type User = ReturnType<typeof userEvent.setup>

// The filters appear once the project index (and the groups' branches) have loaded.
async function chooseBranch(user: User, name: string) {
  const trigger = await screen.findByRole('combobox', { name: /filter by branch/i })
  await waitFor(() => expect(trigger).toBeEnabled())
  await user.click(trigger)
  await user.click(await screen.findByRole('option', { name }))
}

async function toggleGroup(user: User, name: string) {
  await user.click(await screen.findByRole('button', { name: /filter by group/i }))
  await user.click(await screen.findByRole('checkbox', { name }))
}

beforeEach(() => {
  vi.mocked(fetchRunsFeed).mockReset()
  vi.mocked(fetchRunsFeed).mockResolvedValue(feed())
  useUIStore.setState({ selectedBranch: undefined, runsFeedGroupIds: [] })
})

describe('RunsFeedPage URL state', () => {
  it('reads branch, groups and page from the URL', async () => {
    renderFeedAt('/?branch=develop&group=10&group=20&page=2')
    await waitFor(() => {
      expect(lastFeedCall()).toEqual([2, undefined, 'develop', [10, 20]])
    })
  })

  // A bare "/" is the user's own default view: their saved groups, every branch.
  it('seeds the groups of a bare "/" from the saved selection', async () => {
    useUIStore.setState({ runsFeedGroupIds: [20] })
    renderFeedAt('/')
    await waitFor(() => {
      expect(lastFeedCall()).toEqual([1, undefined, undefined, [20]])
    })
  })

  it('lets explicit URL groups win over the saved selection', async () => {
    useUIStore.setState({ runsFeedGroupIds: [20] })
    renderFeedAt('/?group=10')
    await waitFor(() => {
      expect(lastFeedCall()).toEqual([1, undefined, undefined, [10]])
    })
  })

  // Project pages keep the global branch; the feed must neither read nor write it.
  it('ignores the global selectedBranch and leaves it untouched when a branch is chosen', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ selectedBranch: 'main' })
    const router = renderFeedAt('/')

    await waitFor(() => {
      expect(lastFeedCall()).toEqual([1, undefined, undefined, undefined])
    })
    await chooseBranch(user, 'develop')

    expect(router.state.location.search).toBe('?branch=develop')
    expect(useUIStore.getState().selectedBranch).toBe('main')
    await waitFor(() => {
      expect(lastFeedCall()).toEqual([1, undefined, 'develop', undefined])
    })
  })

  it('writes page 2 into the URL and drops it again on page 1', async () => {
    const user = userEvent.setup()
    const router = renderFeedAt('/?branch=main')

    await user.click(await screen.findByRole('button', { name: /next/i }))
    expect(router.state.location.search).toBe('?branch=main&page=2')
    await waitFor(() => {
      expect(lastFeedCall()).toEqual([2, undefined, 'main', undefined])
    })

    await user.click(screen.getByRole('button', { name: /previous/i }))
    expect(router.state.location.search).toBe('?branch=main')
  })

  // Filter state replaces the history entry; only real navigation adds one, so
  // Back from a run page returns to this exact view.
  it('replaces the history entry instead of pushing one', async () => {
    const user = userEvent.setup()
    const router = renderFeedAt('/')

    await user.click(await screen.findByRole('button', { name: /next/i }))
    expect(router.state.location.search).toBe('?page=2')
    expect(router.state.historyAction).toBe('REPLACE')
  })

  it.each([
    { name: 'branch', url: '/?page=3', want: '?branch=develop' },
    { name: 'group', url: '/?branch=main&page=3', want: '?branch=main&group=10' },
  ])('resets to page 1 when the $name changes', async ({ name, url, want }) => {
    const user = userEvent.setup()
    const router = renderFeedAt(url)
    await waitFor(() => expect(lastFeedCall()?.[0]).toBe(3))

    if (name === 'branch') await chooseBranch(user, 'develop')
    else await toggleGroup(user, 'Acme')

    expect(router.state.location.search).toBe(want)
    await waitFor(() => expect(lastFeedCall()?.[0]).toBe(1))
  })

  // The saved selection is still what a bare "/" comes back to next time.
  it('saves a group change for the next bare "/"', async () => {
    const user = userEvent.setup()
    renderFeedAt('/')
    await toggleGroup(user, 'Acme')

    expect(useUIStore.getState().runsFeedGroupIds).toEqual([10])
  })
})

async function clearBranch(user: User) {
  const trigger = await screen.findByRole('combobox', { name: /filter by branch/i })
  await waitFor(() => expect(trigger).toBeEnabled())
  await user.click(trigger)
  await user.click(await screen.findByText(/clear/i))
}

const emptyFeed = (totalPages = 1): PaginatedResponse<PipelineRun[]> => ({
  ...feed(totalPages),
  data: [],
})

describe('RunsFeedPage URL parsing', () => {
  it.each(['Infinity', '1e3', '-2', 'abc', '0', '99999999999999999999'])(
    'reads ?page=%s as page 1',
    async (raw) => {
      renderFeedAt(`/?branch=main&page=${raw}`)
      await waitFor(() => expect(lastFeedCall()).toEqual([1, undefined, 'main', undefined]))
    },
  )

  it('keeps only whole positive group ids, once each', async () => {
    renderFeedAt('/?group=10&group=10&group=1e1&group=abc&group=-3&group=20')
    await waitFor(() => expect(lastFeedCall()).toEqual([1, undefined, undefined, [10, 20]]))
  })

  // A page past the end (an old link, a filter that shrank) moves to the last page once.
  it('moves a page past the end to the last page, once', async () => {
    vi.mocked(fetchRunsFeed).mockImplementation((page = 1) =>
      Promise.resolve(page > 3 ? emptyFeed(3) : feed(3)),
    )
    const router = renderFeedAt('/?page=9')

    await waitFor(() => expect(router.state.location.search).toBe('?page=3'))
    await screen.findByText('Page 3 of 3')
    expect(vi.mocked(fetchRunsFeed).mock.calls.map((c) => c[0])).toEqual([9, 3])
    expect(router.state.historyAction).toBe('REPLACE')
  })
})

describe('RunsFeedPage "all groups" marker', () => {
  // A colleague's /?branch=main shows every group. Clearing the branch must not
  // silently narrow the feed to this user's saved groups.
  it('writes group=all when a change would leave a bare URL over saved groups', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ runsFeedGroupIds: [20] })
    const router = renderFeedAt('/?branch=main')
    await waitFor(() => expect(lastFeedCall()).toEqual([1, undefined, 'main', undefined]))

    await clearBranch(user)

    expect(router.state.location.search).toBe('?group=all')
    await waitFor(() => expect(lastFeedCall()).toEqual([1, undefined, undefined, undefined]))
    expect(useUIStore.getState().runsFeedGroupIds).toEqual([20])
  })

  it('keeps the marker when paging an all-groups view over saved groups', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ runsFeedGroupIds: [20] })
    const router = renderFeedAt('/?branch=main&page=2')

    await user.click(await screen.findByRole('button', { name: /previous/i }))
    expect(router.state.location.search).toBe('?branch=main&group=all')
  })

  it('reads group=all as every group, never as an id, and leaves the saved groups alone', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ runsFeedGroupIds: [20] })
    renderFeedAt('/?group=all')

    await waitFor(() => expect(lastFeedCall()).toEqual([1, undefined, undefined, undefined]))
    expect(await screen.findByRole('button', { name: /filter by group/i })).toHaveTextContent(
      'All groups',
    )
    await chooseBranch(user, 'develop')
    expect(useUIStore.getState().runsFeedGroupIds).toEqual([20])
  })

  it('drops the marker once a group is picked, and saves that group', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ runsFeedGroupIds: [20] })
    const router = renderFeedAt('/?group=all')

    await toggleGroup(user, 'Acme')

    expect(router.state.location.search).toBe('?group=10')
    expect(useUIStore.getState().runsFeedGroupIds).toEqual([10])
  })

  it('needs no marker once the saved groups are cleared too', async () => {
    const user = userEvent.setup()
    useUIStore.setState({ runsFeedGroupIds: [10] })
    const router = renderFeedAt('/')

    await user.click(await screen.findByRole('button', { name: /filter by group/i }))
    await user.click(await screen.findByText(/clear selection/i))

    expect(router.state.location.search).toBe('')
    expect(useUIStore.getState().runsFeedGroupIds).toEqual([])
  })
})

describe('RunsFeedPage empty states', () => {
  it.each([
    { name: 'a branch', url: '/?branch=main', saved: [], href: '/' },
    // Show all runs must beat the saved groups a bare "/" would bring back.
    { name: 'saved groups', url: '/', saved: [20], href: '/?group=all' },
  ])('says no runs match when $name filters the feed empty', async ({ url, saved, href }) => {
    vi.mocked(fetchRunsFeed).mockResolvedValue(emptyFeed())
    useUIStore.setState({ runsFeedGroupIds: saved })
    renderFeedAt(url)

    expect(await screen.findByText('No runs match these filters')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Show all runs' })).toHaveAttribute('href', href)
    expect(screen.queryByRole('link', { name: /browse projects/i })).not.toBeInTheDocument()
  })
})
