import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { SidebarProvider } from '@/components/ui/sidebar'
import { useAuthStore, type Role } from '@/store/auth'
import { AppSidebar } from '../AppSidebar'

vi.mock('@/api/system', () => ({
  getConfig: vi.fn().mockResolvedValue({ data: { mcp_enabled: false }, metadata: {} }),
}))

// SidebarProvider's mobile check needs matchMedia, which jsdom lacks.
vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addEventListener() {},
  removeEventListener() {},
}))

describe('AppSidebar', () => {
  // API Keys is shown to everyone; System Monitor and Users are admin-only,
  // Webhooks editor+ (fix fdfafeb).
  it.each<[Role[], string[]]>([
    [[], ['Runs', 'Projects', 'API Keys']],
    [['editor'], ['Runs', 'Projects', 'API Keys', 'Webhooks']],
    [['admin'], ['Runs', 'Projects', 'System Monitor', 'API Keys', 'Users', 'Webhooks']],
  ])('roles %j see the links %j', (roles, links) => {
    useAuthStore.setState({ roles })
    renderWithProviders(
      <SidebarProvider>
        <AppSidebar />
      </SidebarProvider>,
    )
    expect(screen.getAllByRole('link').map((link) => link.textContent)).toEqual(links)
  })
})
