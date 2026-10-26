import { describe, it, expect, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { useAuthStore } from '@/store/auth'
import { ProjectActionsMenu } from '../ProjectActionsMenu'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 2, slug: 'webshop', display_name: 'webshop', report_type: 'allure' },
      {
        project_id: 3,
        slug: 'webshop-web',
        display_name: 'webshop-web',
        parent_id: 2,
        report_type: 'allure',
      },
    ],
    metadata: { message: 'ok' },
  }),
}))

// Each dialog shows the label it was given, as the real ones put it in their text and toasts.
vi.mock('@/features/reports/SendResultsDialog', () => ({
  SendResultsDialog: ({ open, label }: { open: boolean; label?: string }) =>
    open ? <div data-testid="send-dialog">{label}</div> : null,
}))
vi.mock('@/features/reports/CleanDialog', () => ({
  CleanDialog: ({ open, mode, label }: { open: boolean; mode: string; label?: string }) =>
    open ? <div data-testid={`clean-dialog-${mode}`}>{label}</div> : null,
}))

// The numeric project id is an internal key; every dialog names the project by its label.
describe('ProjectActionsMenu dialog labels', () => {
  it.each<[string, string]>([
    ['Send results', 'send-dialog'],
    ['Clean results', 'clean-dialog-results'],
    ['Clean history', 'clean-dialog-history'],
  ])('passes the %s dialog the project label, not the numeric id', async (action, testId) => {
    useAuthStore.setState({ roles: ['admin'] })
    renderWithProviders(
      <Routes>
        <Route path="/projects/:id/*" element={<ProjectActionsMenu />} />
      </Routes>,
      { route: '/projects/3' },
    )

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /project actions/i }))
    await user.click(await screen.findByRole('menuitem', { name: action }))

    await waitFor(() => expect(screen.getByTestId(testId)).toHaveTextContent('webshop/webshop-web'))
  })
})
