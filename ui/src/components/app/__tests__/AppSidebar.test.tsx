import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SidebarProvider } from '@/components/ui/sidebar'
import { AppSidebar } from '../AppSidebar'

// Mock sub-components that haven't been implemented yet / have their own tests
vi.mock('../ProjectSwitcher', () => ({
  ProjectSwitcher: () => <div data-testid="project-switcher">ProjectSwitcher</div>,
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

// matchMedia is required by useIsMobile
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
            <Route path="/projects/:id/*" element={<AppSidebar />} />
          </Routes>
        </SidebarProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('AppSidebar', () => {
  it('renders project switcher', () => {
    renderSidebar('/')
    expect(screen.getByTestId('project-switcher')).toBeInTheDocument()
  })

  it('renders search trigger', () => {
    renderSidebar('/')
    expect(screen.getByTestId('search-trigger')).toBeInTheDocument()
  })

  it('hides nav items when no project is selected', () => {
    renderSidebar('/')
    expect(screen.queryByText('Overview')).not.toBeInTheDocument()
    expect(screen.queryByText('Known Issues')).not.toBeInTheDocument()
    expect(screen.queryByText('Timeline')).not.toBeInTheDocument()
    expect(screen.queryByText('Analytics')).not.toBeInTheDocument()
  })

  it('shows nav items when a project is selected', () => {
    renderSidebar('/projects/my-project')
    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(screen.getByText('Known Issues')).toBeInTheDocument()
    expect(screen.getByText('Timeline')).toBeInTheDocument()
    expect(screen.getByText('Analytics')).toBeInTheDocument()
  })

  it('nav items link to correct project paths', () => {
    renderSidebar('/projects/my-project')
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
