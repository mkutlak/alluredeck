import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { useAuthStore, type Role } from '@/store/auth'
import { ProjectActionsMenu } from '../ProjectActionsMenu'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 7, slug: 'my-project', report_type: 'allure' }],
    metadata: { message: 'ok' },
  }),
}))

vi.mock('@/features/reports/SendResultsDialog', () => ({
  SendResultsDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="send-dialog">SendDialog</div> : null,
}))

vi.mock('@/features/reports/CleanDialog', () => ({
  CleanDialog: ({ open, mode }: { open: boolean; mode: string }) =>
    open ? <div data-testid={`clean-dialog-${mode}`}>CleanDialog</div> : null,
}))

function renderMenu(path: string, roles: Role[]) {
  useAuthStore.setState({ roles })
  return renderWithProviders(
    <Routes>
      <Route path="/" element={<ProjectActionsMenu />} />
      <Route path="/projects/:id/*" element={<ProjectActionsMenu />} />
    </Routes>,
    { route: path },
  )
}

async function openMenu() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /project actions/i }))
  await screen.findAllByRole('menuitem')
  return user
}

describe('ProjectActionsMenu', () => {
  it.each<[string, string, Role[]]>([
    ['no project is in the URL', '/', ['admin']],
    ['the user is a viewer', '/projects/7', ['viewer']],
  ])('renders nothing when %s', (_, path, roles) => {
    renderMenu(path, roles)
    expect(screen.queryByRole('button', { name: /project actions/i })).not.toBeInTheDocument()
  })

  // Sending results needs editor; cleaning is admin-only.
  it.each<[Role, string[]]>([
    ['admin', ['Send results', 'Clean results', 'Clean history']],
    ['editor', ['Send results']],
  ])('offers %s the actions %j', async (role, actions) => {
    renderMenu('/projects/7', [role])
    await openMenu()
    expect(screen.getAllByRole('menuitem').map((item) => item.textContent)).toEqual(actions)
  })

  it.each([
    ['Send results', 'send-dialog'],
    ['Clean results', 'clean-dialog-results'],
    ['Clean history', 'clean-dialog-history'],
  ])('opens the matching dialog for "%s"', async (action, dialogTestId) => {
    renderMenu('/projects/7', ['admin'])
    const user = await openMenu()
    await user.click(screen.getByRole('menuitem', { name: action }))
    expect(screen.getByTestId(dialogTestId)).toBeInTheDocument()
  })
})
