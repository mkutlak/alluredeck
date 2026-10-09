import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, useLocation } from 'react-router'
import { renderWithProviders } from '@/test/render'
import * as searchApi from '@/api/search'
import * as projectsApi from '@/api/projects'
import { SearchCommand } from '../SearchCommand'
import type { SearchData } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/search')
vi.mock('@/api/projects')
mockApiClient()

const run = (pipeline_id: string, branch = 'main') => ({
  group_project_id: 10,
  group_slug: 'checkout',
  pipeline_id,
  branch,
  timestamp: '2026-05-01T10:00:00Z',
})
const aProject = { project_id: 1, slug: 'my-project', created_at: '2026-01-01T00:00:00Z' }

function Where() {
  const { pathname, search } = useLocation()
  return <p data-testid="where">{pathname + search}</p>
}

async function openAndSearch(data: Partial<SearchData>) {
  const user = userEvent.setup()
  vi.mocked(searchApi.search).mockResolvedValue({
    data: { projects: [], tests: [], ...data },
    metadata: { message: 'Search results' },
  })
  const router = createMemoryRouter([
    {
      path: '*',
      element: (
        <>
          <SearchCommand />
          <Where />
        </>
      ),
    },
  ])
  renderWithProviders(<></>, { router })
  await user.keyboard('{Meta>}k{/Meta}')
  await user.type(screen.getByPlaceholderText(/search projects/i), 'query')
  return user
}

describe('SearchCommand runs and branches', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
  })

  it('invites a search for runs and branches in the placeholder', async () => {
    await openAndSearch({})
    expect(screen.getByPlaceholderText(/runs and branches/i)).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/search projects/i)).toBeInTheDocument()
  })

  it('lists a run with its pipeline id, branch and group, and opens the run page', async () => {
    const user = await openAndSearch({ runs: [run('48213', 'feature/login-retry')] })

    const option = await screen.findByRole('option', { name: /48213/ })
    expect(screen.getByText('Runs')).toBeInTheDocument()
    expect(option).toHaveTextContent('feature/login-retry')
    expect(option).toHaveTextContent('checkout')

    await user.click(option)
    expect(screen.getByTestId('where')).toHaveTextContent(/^\/runs\/10\/48213$/)
  })

  it('says "no branch" for a run without one', async () => {
    await openAndSearch({ runs: [run('48213', '')] })
    expect(await screen.findByRole('option', { name: /48213/ })).toHaveTextContent('no branch')
  })

  it("names a run's group by its display name, as the feed does", async () => {
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [{ project_id: 10, slug: 'grp-checkout-01', display_name: 'Checkout Service' }],
      metadata: { message: 'ok' },
    })
    await openAndSearch({ runs: [{ ...run('48213'), group_slug: 'grp-checkout-01' }] })

    const option = await screen.findByRole('option', { name: /48213/ })
    await waitFor(() => expect(option).toHaveTextContent('Checkout Service'))
    expect(option).not.toHaveTextContent('grp-checkout-01')
  })

  it("falls back to the group's slug while the project index has no match", async () => {
    await openAndSearch({ runs: [{ ...run('48213'), group_slug: 'grp-checkout-01' }] })

    expect(await screen.findByRole('option', { name: /48213/ })).toHaveTextContent(
      'grp-checkout-01',
    )
  })

  it('encodes a pipeline id that is free text', async () => {
    const user = await openAndSearch({ runs: [run('gh/run#7')] })

    await user.click(await screen.findByRole('option', { name: /gh\/run#7/ }))
    expect(screen.getByTestId('where')).toHaveTextContent(/^\/runs\/10\/gh%2Frun%237$/)
  })

  it('lists a branch and opens the feed filtered to it', async () => {
    const user = await openAndSearch({ branches: [{ name: 'login-retry' }] })

    const option = await screen.findByRole('option', { name: /login-retry/ })
    expect(screen.getByText('Branches')).toBeInTheDocument()

    await user.click(option)
    expect(screen.getByTestId('where')).toHaveTextContent(/^\/\?branch=login-retry$/)
  })

  it('encodes a branch name with a slash, an ampersand and a space', async () => {
    const user = await openAndSearch({ branches: [{ name: 'fix/a&b c' }] })

    await user.click(await screen.findByRole('option', { name: /fix\/a&b c/ }))
    // The meaning, not the spelling of a space: the feed reads it back with URLSearchParams.
    const url = new URL(screen.getByTestId('where').textContent ?? '', 'http://localhost')
    expect(url.pathname).toBe('/')
    expect(url.searchParams.get('branch')).toBe('fix/a&b c')
  })

  it.each([
    { name: 'absent', extra: {} },
    { name: 'empty', extra: { runs: [], branches: [] } },
  ])('shows no runs or branches group when they are $name', async ({ extra }) => {
    await openAndSearch({ projects: [aProject], ...extra })

    expect(await screen.findByText('my-project')).toBeInTheDocument()
    expect(screen.queryByText('Runs')).not.toBeInTheDocument()
    expect(screen.queryByText('Branches')).not.toBeInTheDocument()
  })

  it('still says there are no results when every group is empty', async () => {
    await openAndSearch({ runs: [], branches: [] })
    expect(await screen.findByText(/no results found/i)).toBeInTheDocument()
  })
})
