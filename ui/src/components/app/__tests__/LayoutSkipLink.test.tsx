import { fireEvent, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { Layout } from '../Layout'

vi.mock('../TopBar', () => ({ TopBar: () => <button type="button">menu</button> }))
vi.mock('../BreadcrumbBar', () => ({ BreadcrumbBar: () => null }))
vi.mock('../AppSidebar', () => ({ AppSidebar: () => null }))
vi.mock('@/hooks/usePreferencesSync', () => ({ usePreferencesSync: () => undefined }))
vi.mock('@/features/search', () => ({
  SearchCommand: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

// SidebarProvider's mobile check needs matchMedia, which jsdom lacks.
vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addEventListener() {},
  removeEventListener() {},
}))

function renderLayout() {
  renderWithProviders(
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<p>page text</p>} />
      </Route>
    </Routes>,
  )
}

describe('Layout skip link', () => {
  // Without it, every page starts with the top bar, the breadcrumbs and the sidebar.
  it('is the first stop for Tab, and Enter moves focus to the page content', async () => {
    const user = userEvent.setup()
    renderLayout()

    await user.tab()
    expect(screen.getByRole('link', { name: 'Skip to content' })).toHaveFocus()

    await user.keyboard('{Enter}')
    expect(document.activeElement).toHaveAttribute('id', 'main-content')
    expect(screen.getByRole('main')).toContainElement(document.activeElement as HTMLElement)
    // No #main-content in the URL: it would replace a run page deep link's #suite hash.
    expect(window.location.hash).toBe('')
  })

  // A focusable page container takes focus on every click into blank space, and the
  // next Tab then starts from the top of the page instead of where the user clicked.
  it('makes the content focusable only while the skip link has sent focus there', async () => {
    const user = userEvent.setup()
    renderLayout()

    await user.click(screen.getByText('page text'))
    expect(document.body).toHaveFocus()

    await user.click(screen.getByRole('link', { name: 'Skip to content' }))
    const content = document.activeElement
    expect(content).toHaveAttribute('id', 'main-content')
    expect(content).toHaveAttribute('tabindex', '-1')

    await user.tab()
    expect(content).not.toHaveFocus()
    expect(content).not.toHaveAttribute('tabindex')
    await user.click(screen.getByText('page text'))
    expect(document.body).toHaveFocus()
  })

  it('leaves no tabindex behind when focus does not land', () => {
    renderLayout()
    const focus = vi.spyOn(HTMLElement.prototype, 'focus').mockImplementation(() => {})
    fireEvent.click(screen.getByRole('link', { name: 'Skip to content' }))
    focus.mockRestore()
    expect(document.getElementById('main-content')).not.toHaveAttribute('tabindex')
  })
})
