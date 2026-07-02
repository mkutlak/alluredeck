import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { GroupFilter } from '../GroupFilter'
import { useUIStore } from '@/store/ui'

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [
      { project_id: 10, slug: 'acme', display_name: 'Acme', parent_id: null, children: [1, 2] },
      { project_id: 20, slug: 'globex', parent_id: null, children: [3] },
      { project_id: 1, slug: 'api-cloud', parent_id: 10, children: [] },
      { project_id: 30, slug: 'standalone', parent_id: null, children: [] },
    ],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

describe('GroupFilter', () => {
  beforeEach(() => {
    useUIStore.setState({ runsFeedGroupIds: [] })
  })

  it('shows "All groups" when nothing is selected', async () => {
    renderWithProviders(<GroupFilter />)
    expect(await screen.findByText('All groups')).toBeInTheDocument()
  })

  it('lists only parent projects (entries with children)', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GroupFilter />)
    await user.click(await screen.findByRole('button'))

    expect(screen.getByText('Acme')).toBeInTheDocument()
    expect(screen.getByText('globex')).toBeInTheDocument()
    expect(screen.queryByText('api-cloud')).not.toBeInTheDocument()
    expect(screen.queryByText('standalone')).not.toBeInTheDocument()
  })

  it('checking a group writes it to runsFeedGroupIds', async () => {
    const user = userEvent.setup()
    renderWithProviders(<GroupFilter />)
    await user.click(await screen.findByRole('button'))
    await user.click(screen.getByText('Acme'))

    expect(useUIStore.getState().runsFeedGroupIds).toEqual([10])
  })

  it('unchecking a selected group removes it from runsFeedGroupIds', async () => {
    useUIStore.setState({ runsFeedGroupIds: [10, 20] })
    const user = userEvent.setup()
    renderWithProviders(<GroupFilter />)
    await user.click(await screen.findByRole('button'))
    await user.click(screen.getByText('Acme'))

    expect(useUIStore.getState().runsFeedGroupIds).toEqual([20])
  })

  it('shows a selection count when groups are selected', async () => {
    useUIStore.setState({ runsFeedGroupIds: [10] })
    renderWithProviders(<GroupFilter />)
    expect(await screen.findByText('1 group selected')).toBeInTheDocument()
  })

  it('pluralizes the selection count for multiple groups', async () => {
    useUIStore.setState({ runsFeedGroupIds: [10, 20] })
    renderWithProviders(<GroupFilter />)
    expect(await screen.findByText('2 groups selected')).toBeInTheDocument()
  })
})
