import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import * as searchApi from '@/api/search'
import * as projectsApi from '@/api/projects'
import { SearchCommand } from '../SearchCommand'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/search')
vi.mock('@/api/projects')
mockApiClient()

function renderSearch() {
  return renderWithProviders(<SearchCommand />)
}

describe('SearchCommand', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: empty project index (no groups)
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
  })

  it('opens dialog on Cmd+K', async () => {
    const user = userEvent.setup()
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    expect(screen.getByPlaceholderText(/search projects/i)).toBeInTheDocument()
  })

  it('shows empty state when no results', async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: { projects: [], tests: [] },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'nonexistent')

    await waitFor(() => {
      expect(screen.getByText(/no results/i)).toBeInTheDocument()
    })
  })

  it('displays project results', async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: {
        projects: [{ project_id: 1, slug: 'my-project', created_at: '2026-01-01T00:00:00Z' }],
        tests: [],
      },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'my')

    await waitFor(() => {
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
  })

  it('displays test results with status badge', async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: {
        projects: [],
        tests: [
          {
            project_id: 1,
            slug: 'my-project',
            test_name: 'LoginTest',
            full_name: 'com.auth.LoginTest',
            status: 'passed',
          },
        ],
      },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'login')

    await waitFor(() => {
      expect(screen.getByText('LoginTest')).toBeInTheDocument()
    })
  })

  it('shows Folder icon for group project results', async () => {
    const user = userEvent.setup()
    // Project 1 is a group (has children)
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [
        { project_id: 1, slug: 'parent-project', children: [2] },
        { project_id: 2, slug: 'child-project', parent_id: 1 },
      ],
      metadata: { message: 'ok' },
    })
    vi.mocked(searchApi.search).mockResolvedValue({
      data: {
        projects: [{ project_id: 1, slug: 'parent-project', created_at: '2026-01-01T00:00:00Z' }],
        tests: [],
      },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'parent')

    await waitFor(() => {
      expect(screen.getByText('parent-project')).toBeInTheDocument()
    })
    expect(document.querySelector('[data-testid="icon-folder"]')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="icon-file-text"]')).not.toBeInTheDocument()
  })

  it('shows FileText icon for leaf project results', async () => {
    const user = userEvent.setup()
    // Project 2 is a leaf (no children)
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [
        { project_id: 1, slug: 'parent-project', children: [2] },
        { project_id: 2, slug: 'child-project', parent_id: 1 },
      ],
      metadata: { message: 'ok' },
    })
    vi.mocked(searchApi.search).mockResolvedValue({
      data: {
        projects: [{ project_id: 2, slug: 'child-project', created_at: '2026-01-01T00:00:00Z' }],
        tests: [],
      },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'child')

    await waitFor(() => {
      expect(screen.getByText('child-project')).toBeInTheDocument()
    })
    expect(document.querySelector('[data-testid="icon-file-text"]')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="icon-folder"]')).not.toBeInTheDocument()
  })
})
