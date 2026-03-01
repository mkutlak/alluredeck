import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SidebarProvider } from '@/components/ui/sidebar'
import { AppSidebar } from '../AppSidebar'

vi.mock('@/api/projects', () => ({
  getProjects: vi.fn().mockResolvedValue({
    data: [{ project_id: 'project-alpha' }, { project_id: 'my-project' }],
    metadata: { message: 'ok', total: 2, page: 1, per_page: 20, total_pages: 1 },
  }),
}))

vi.mock('@/features/search', () => ({
  SearchTrigger: () => <div data-testid="search-trigger">SearchTrigger</div>,
  SearchCommand: () => null,
  GlobalSearch: () => null,
}))

vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

function renderSidebar(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={qc}>
        <SidebarProvider>
          <Routes>
            <Route path="/" element={<AppSidebar />} />
            <Route path="/dashboard" element={<AppSidebar />} />
            <Route path="/projects/:id/*" element={<AppSidebar />} />
          </Routes>
        </SidebarProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('AppSidebar', () => {
  it('renders dashboard link', () => {
    renderSidebar('/')
    const link = screen.getByRole('link', { name: /projects dashboard/i })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/')
  })

  it('renders search trigger', () => {
    renderSidebar('/')
    expect(screen.getByTestId('search-trigger')).toBeInTheDocument()
  })

  it('renders collapsible Projects trigger button', () => {
    renderSidebar('/')
    expect(screen.getByRole('button', { name: /projects/i })).toBeInTheDocument()
  })

  it('shows project list when collapsible is open by default', async () => {
    renderSidebar('/')
    await waitFor(() => {
      expect(screen.getByText('project-alpha')).toBeInTheDocument()
      expect(screen.getByText('my-project')).toBeInTheDocument()
    })
  })

  it('collapses and expands project list on trigger click', async () => {
    const user = userEvent.setup()
    renderSidebar('/')
    await waitFor(() => expect(screen.getByText('project-alpha')).toBeInTheDocument())

    const trigger = screen.getByRole('button', { name: /projects/i })
    await user.click(trigger)
    expect(screen.queryByText('project-alpha')).not.toBeInTheDocument()

    await user.click(trigger)
    expect(screen.getByText('project-alpha')).toBeInTheDocument()
  })

  it('highlights active project with data-active', async () => {
    renderSidebar('/projects/my-project')
    await waitFor(() => {
      // "my-project" appears in both the project list link and the section label;
      // only the project list link carries data-active="true"
      const allMatches = screen.getAllByText('my-project')
      const activeEl = allMatches.find((el) => el.closest('[data-active="true"]'))
      expect(activeEl).toBeInTheDocument()
    })
  })

  it('shows nav items under active project', async () => {
    renderSidebar('/projects/my-project')
    await waitFor(() => {
      expect(screen.getByText('Overview')).toBeInTheDocument()
      expect(screen.getByText('Known Issues')).toBeInTheDocument()
      expect(screen.getByText('Timeline')).toBeInTheDocument()
      expect(screen.getByText('Analytics')).toBeInTheDocument()
    })
  })

  it('hides nav items when no project is selected', async () => {
    renderSidebar('/')
    await waitFor(() => expect(screen.getByText('project-alpha')).toBeInTheDocument())
    expect(screen.queryByText('Overview')).not.toBeInTheDocument()
    expect(screen.queryByText('Known Issues')).not.toBeInTheDocument()
    expect(screen.queryByText('Timeline')).not.toBeInTheDocument()
    expect(screen.queryByText('Analytics')).not.toBeInTheDocument()
  })

  it('nav items link to correct project paths', async () => {
    renderSidebar('/projects/my-project')
    await waitFor(() => {
      expect(screen.getByRole('link', { name: /overview/i })).toHaveAttribute(
        'href',
        '/projects/my-project',
      )
      expect(screen.getByRole('link', { name: /known issues/i })).toHaveAttribute(
        'href',
        '/projects/my-project/known-issues',
      )
      expect(screen.getByRole('link', { name: /timeline/i })).toHaveAttribute(
        'href',
        '/projects/my-project/timeline',
      )
      expect(screen.getByRole('link', { name: /analytics/i })).toHaveAttribute(
        'href',
        '/projects/my-project/analytics',
      )
    })
  })

  it('shows project sub-nav as separate section when project is selected', async () => {
    renderSidebar('/projects/my-project')
    await waitFor(() => {
      expect(screen.getByText('Overview')).toBeInTheDocument()
    })
    const overviewLink = screen.getByRole('link', { name: /overview/i })
    // Nav items must NOT be nested inside the collapsible project list (menu-sub)
    // They live in their own separate SidebarGroup (Section 4)
    expect(overviewLink.closest('[data-sidebar="menu-sub"]')).toBeNull()
  })
})
