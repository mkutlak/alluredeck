import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useAuthStore } from '@/store/auth'
import { CreateMenu } from '../CreateMenu'

vi.mock('@/features/projects/CreateProjectDialog', () => ({
  CreateProjectDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="create-project-dialog">Dialog</div> : null,
}))

describe('CreateMenu', () => {
  it('returns null when user is not admin', () => {
    useAuthStore.setState({ roles: ['editor'] })
    const { container } = render(<CreateMenu />)
    expect(container).toBeEmptyDOMElement()
  })

  it('clicking "New Project" opens the CreateProjectDialog', async () => {
    const user = userEvent.setup()
    useAuthStore.setState({ roles: ['admin'] })
    render(<CreateMenu />)

    await user.click(screen.getByRole('button', { name: /create new/i }))
    await user.click(screen.getByRole('menuitem', { name: /new project/i }))

    expect(screen.getByTestId('create-project-dialog')).toBeInTheDocument()
  })
})
