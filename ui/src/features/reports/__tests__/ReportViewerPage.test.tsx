import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ReportViewerPage } from '../ReportViewerPage'

// Mock useQuery so we can control which project/report_type is returned per test
const mockUseQuery = vi.fn()

vi.mock('@tanstack/react-query', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-query')>('@tanstack/react-query')
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
  }
})

function renderPage(
  projectId = 'my-project',
  reportId = '3',
  reportType: 'allure' | 'playwright' = 'allure',
) {
  mockUseQuery.mockReturnValue({
    data: {
      data: [{ project_id: 1, slug: projectId, report_type: reportType }],
    },
    isLoading: false,
    error: null,
  })

  const router = createMemoryRouter(
    [{ path: '/projects/:id/reports/:reportId', element: <ReportViewerPage /> }],
    { initialEntries: [`/projects/${projectId}/reports/${reportId}`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('ReportViewerPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders an iframe pointing at the correct report URL', () => {
    renderPage()
    const iframe = screen.getByTitle(/allure report/i)
    expect(iframe).toBeInTheDocument()
    expect(iframe).toHaveAttribute(
      'src',
      expect.stringContaining('/projects/my-project/reports/3/index.html'),
    )
  })

  it('iframe sandbox allows downloads so attachment links work', () => {
    renderPage()
    const iframe = screen.getByTitle(/allure report/i)
    const sandbox = iframe.getAttribute('sandbox') ?? ''
    expect(sandbox).toContain('allow-downloads')
  })

  it('iframe sandbox retains existing permissions', () => {
    renderPage()
    const iframe = screen.getByTitle(/allure report/i)
    const sandbox = iframe.getAttribute('sandbox') ?? ''
    expect(sandbox).toContain('allow-scripts')
    expect(sandbox).toContain('allow-same-origin')
    expect(sandbox).toContain('allow-popups')
    expect(sandbox).toContain('allow-forms')
  })

  describe('view mode toggle', () => {
    it('toggle renders for playwright projects', () => {
      renderPage('pw-project', '5', 'playwright')
      expect(screen.getByRole('button', { name: /playwright/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /allure/i })).toBeInTheDocument()
    })

    it('toggle does NOT render for allure-only projects', () => {
      renderPage('allure-project', '5', 'allure')
      expect(screen.queryByRole('button', { name: /playwright/i })).not.toBeInTheDocument()
      // The Allure toggle button is absent — only the "Open in new tab" link exists
      expect(screen.queryByRole('button', { name: /^allure$/i })).not.toBeInTheDocument()
    })

    it('default view mode is playwright for playwright projects', () => {
      renderPage('pw-project', '5', 'playwright')
      const iframe = screen.getByTitle(/playwright report/i)
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('/playwright-reports/5/index.html'),
      )
      const playwrightBtn = screen.getByRole('button', { name: /playwright/i })
      expect(playwrightBtn).toHaveAttribute('aria-pressed', 'true')
    })

    it('default view mode is allure for allure projects', () => {
      renderPage('allure-project', '5', 'allure')
      const iframe = screen.getByTitle(/allure report/i)
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('/reports/5/index.html'),
      )
    })

    it('iframe URL changes to allure URL when allure toggle is clicked', async () => {
      const user = userEvent.setup()
      renderPage('pw-project', '5', 'playwright')

      // Initially shows playwright URL
      expect(screen.getByTitle(/playwright report/i)).toHaveAttribute(
        'src',
        expect.stringContaining('/playwright-reports/5/index.html'),
      )

      await user.click(screen.getByRole('button', { name: /allure/i }))

      // After clicking Allure, iframe should point to allure URL
      const iframe = screen.getByTitle(/allure report/i)
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('/reports/5/index.html'),
      )
      expect(iframe).not.toHaveAttribute(
        'src',
        expect.stringContaining('/playwright-reports/'),
      )
    })

    it('iframe URL changes back to playwright URL when playwright toggle is clicked', async () => {
      const user = userEvent.setup()
      renderPage('pw-project', '5', 'playwright')

      // Switch to allure first
      await user.click(screen.getByRole('button', { name: /allure/i }))
      expect(screen.getByTitle(/allure report/i)).toHaveAttribute(
        'src',
        expect.stringContaining('/reports/5/index.html'),
      )

      // Switch back to playwright
      await user.click(screen.getByRole('button', { name: /playwright/i }))
      const iframe = screen.getByTitle(/playwright report/i)
      expect(iframe).toHaveAttribute(
        'src',
        expect.stringContaining('/playwright-reports/5/index.html'),
      )
    })

    it('Open in new tab link reflects current view mode URL', async () => {
      const user = userEvent.setup()
      renderPage('pw-project', '5', 'playwright')

      // Default: playwright URL in "Open in new tab"
      const link = screen.getByRole('link', { name: /open in new tab/i })
      expect(link).toHaveAttribute('href', expect.stringContaining('/playwright-reports/5/index.html'))

      // Switch to allure
      await user.click(screen.getByRole('button', { name: /allure/i }))
      expect(link).toHaveAttribute('href', expect.stringContaining('/reports/5/index.html'))
    })

    it('allure button shows aria-pressed true after clicking allure', async () => {
      const user = userEvent.setup()
      renderPage('pw-project', '5', 'playwright')

      const allureBtn = screen.getByRole('button', { name: /allure/i })
      expect(allureBtn).toHaveAttribute('aria-pressed', 'false')

      await user.click(allureBtn)
      expect(allureBtn).toHaveAttribute('aria-pressed', 'true')

      const playwrightBtn = screen.getByRole('button', { name: /playwright/i })
      expect(playwrightBtn).toHaveAttribute('aria-pressed', 'false')
    })
  })
})
