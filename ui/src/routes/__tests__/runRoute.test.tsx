import { screen } from '@testing-library/react'
import { Outlet, useLocation, useParams } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { AppRoutes } from '..'

// Only the route table is under test: auth, the shell and the pages are stand-ins.
vi.mock('@/features/auth/AuthGuard', () => ({
  AuthGuard: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/components/app/Layout', () => ({ Layout: () => <Outlet /> }))
vi.mock('@/features/runs', () => ({
  RunsFeedPage: () => <p>runs feed</p>,
  RunPage: function RunPage() {
    const { groupId, runKey } = useParams()
    return <p>{`run page ${groupId} ${runKey}${useLocation().hash}`}</p>
  },
}))

describe('the run route', () => {
  it.each([
    { url: '/runs/10/48213', text: 'run page 10 48213' },
    // The key is encoded in links; the page receives it decoded.
    { url: '/runs/10/gh%2Frun%207', text: 'run page 10 gh/run 7' },
    { url: '/runs/10/48213#suite-3', text: 'run page 10 48213#suite-3' },
  ])('renders the run page for $url', async ({ url, text }) => {
    renderWithProviders(<AppRoutes />, { route: url })
    expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it('still renders the feed at "/"', async () => {
    renderWithProviders(<AppRoutes />, { route: '/?branch=main' })
    expect(await screen.findByText('runs feed')).toBeInTheDocument()
  })
})
