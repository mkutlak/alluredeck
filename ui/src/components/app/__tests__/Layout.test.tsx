import { screen } from '@testing-library/react'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { useSidebar } from '@/components/ui/sidebar'
import { Layout } from '../Layout'

// Only the provider wiring is under test; the chrome around it is not.
vi.mock('../TopBar', () => ({ TopBar: () => null }))
vi.mock('../BreadcrumbBar', () => ({ BreadcrumbBar: () => null }))
vi.mock('@/hooks/usePreferencesSync', () => ({ usePreferencesSync: () => undefined }))
vi.mock('@/features/search', () => ({
  SearchCommand: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('../AppSidebar', () => ({
  AppSidebar: () => <div data-testid="sidebar-state">{useSidebar().state}</div>,
}))

// SidebarProvider's mobile check needs matchMedia, which jsdom lacks.
vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addEventListener() {},
  removeEventListener() {},
}))

afterEach(() => {
  document.cookie = 'sidebar_state=; max-age=0; path=/'
})

describe('Layout', () => {
  it.each([
    { cookie: undefined, state: 'expanded' },
    { cookie: 'sidebar_state=false', state: 'collapsed' },
    { cookie: 'sidebar_state=true', state: 'expanded' },
  ])('starts the sidebar $state when the cookie is $cookie', ({ cookie, state }) => {
    if (cookie) document.cookie = `${cookie}; path=/`
    renderWithProviders(<Layout />)
    expect(screen.getByTestId('sidebar-state')).toHaveTextContent(state)
  })
})
