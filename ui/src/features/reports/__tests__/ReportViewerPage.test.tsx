import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { ReportViewerPage } from '../ReportViewerPage'

function renderPage(projectId = 'my-project', reportId = '3') {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/reports/:reportId', element: <ReportViewerPage /> }],
    { initialEntries: [`/projects/${projectId}/reports/${reportId}`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('ReportViewerPage', () => {
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
})
