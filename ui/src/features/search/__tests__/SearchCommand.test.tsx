import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import * as searchApi from '@/api/search'
import * as projectsApi from '@/api/projects'
import { SearchCommand } from '../SearchCommand'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/search')
vi.mock('@/api/projects')
mockApiClient()

// Project 1 is a group (has children); project 2 is a leaf.
const index = [
  { project_id: 1, slug: 'parent-project', children: [2] },
  { project_id: 2, slug: 'child-project', parent_id: 1 },
]
const project = (project_id: number, slug: string) => ({
  project_id,
  slug,
  created_at: '2026-01-01T00:00:00Z',
})
const loginTest = {
  project_id: 1,
  slug: 'my-project',
  test_name: 'LoginTest',
  full_name: 'com.auth.LoginTest',
  status: 'passed',
}

describe('SearchCommand', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: index,
      metadata: { message: 'ok' },
    })
  })

  it.each([
    { name: 'no results', projects: [], tests: [], see: /no results/i, icon: null },
    { name: 'a test result', projects: [], tests: [loginTest], see: 'LoginTest', icon: null },
    {
      name: 'a group project',
      projects: [project(1, 'parent-project')],
      tests: [],
      see: 'parent-project',
      icon: 'icon-folder',
    },
    {
      name: 'a leaf project',
      projects: [project(2, 'child-project')],
      tests: [],
      see: 'child-project',
      icon: 'icon-file-text',
    },
  ])('opens on Cmd+K and shows $name', async ({ projects, tests, see, icon }) => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: { projects, tests },
      metadata: { message: 'Search results' },
    })
    renderWithProviders(<SearchCommand />)

    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'query')

    expect(await screen.findByText(see)).toBeInTheDocument()
    // Groups get a folder icon, leaf projects a file icon.
    for (const id of ['icon-folder', 'icon-file-text']) {
      expect(screen.queryByTestId(id) !== null).toBe(id === icon)
    }
  })
})
