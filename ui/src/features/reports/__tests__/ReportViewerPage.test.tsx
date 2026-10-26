import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ReportViewerPage } from '../ReportViewerPage'

// Stub useQuery so each test controls the project's report_type.
const mockUseQuery = vi.fn()

vi.mock('@tanstack/react-query', async () => {
  const actual =
    await vi.importActual<typeof import('@tanstack/react-query')>('@tanstack/react-query')
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
  }
})

function renderPage(
  reportType: 'allure' | 'playwright',
  projects: Record<string, unknown>[] = [{ project_id: 1, slug: 'proj', report_type: reportType }],
) {
  mockUseQuery.mockReturnValue({
    data: { data: projects },
    isLoading: false,
    error: null,
  })
  const router = createMemoryRouter(
    [{ path: '/projects/:id/reports/:reportId', element: <ReportViewerPage /> }],
    { initialEntries: ['/projects/proj/reports/5'] },
  )
  return renderWithProviders(<></>, { router })
}

const iframe = () => screen.getByTitle(/report #5/i)
const newTabLink = () => screen.getByRole('link', { name: /open in new tab/i })
const ALLURE_URL = '/projects/proj/reports/5/index.html'
const PLAYWRIGHT_URL = '/projects/proj/playwright-reports/5/index.html'

describe('ReportViewerPage', () => {
  // The embedded report is third-party HTML: the sandbox must keep its exact
  // capabilities (allow-downloads so attachment links work).
  it('sandboxes the report iframe', () => {
    renderPage('allure')
    expect(iframe().getAttribute('sandbox')?.split(' ').sort()).toEqual([
      'allow-downloads',
      'allow-forms',
      'allow-popups',
      'allow-same-origin',
      'allow-scripts',
    ])
  })

  // The view toggle exists only for playwright projects.
  it.each<[string, 'allure' | 'playwright', string, boolean]>([
    ['default view mode is allure for allure projects', 'allure', ALLURE_URL, false],
    ['default view mode is playwright for playwright projects', 'playwright', PLAYWRIGHT_URL, true],
  ])('%s', (_name, type, url, toggle) => {
    renderPage(type)

    expect(iframe()).toHaveAttribute('src', expect.stringContaining(url))
    expect(newTabLink()).toHaveAttribute('href', expect.stringContaining(url))
    expect(!!screen.queryByRole('button', { name: /playwright/i })).toBe(toggle)
  })

  it('toggles a playwright project between the playwright and allure reports', async () => {
    const user = userEvent.setup()
    renderPage('playwright')

    await user.click(screen.getByRole('button', { name: /allure/i }))
    expect(iframe()).toHaveAttribute('src', expect.stringContaining(ALLURE_URL))
    expect(newTabLink()).toHaveAttribute('href', expect.stringContaining(ALLURE_URL))

    await user.click(screen.getByRole('button', { name: /playwright/i }))
    expect(iframe()).toHaveAttribute('src', expect.stringContaining(PLAYWRIGHT_URL))
  })

  // The iframe title is what screen readers announce, so it names the project like the toolbar does.
  it('titles the report iframe with the project label, not its id', () => {
    renderPage('allure', [
      { project_id: 2, slug: 'webshop', display_name: 'webshop' },
      { project_id: 1, slug: 'proj', display_name: 'webshop-web', parent_id: 2 },
    ])
    expect(iframe()).toHaveAttribute('title', 'Allure report #5 — webshop/webshop-web')
  })
})
