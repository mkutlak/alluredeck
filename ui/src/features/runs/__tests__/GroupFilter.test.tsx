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

async function openFilter() {
  const user = userEvent.setup()
  renderWithProviders(<GroupFilter />)
  await user.click(await screen.findByRole('button'))
  return user
}

describe('GroupFilter', () => {
  beforeEach(() => {
    useUIStore.setState({ runsFeedGroupIds: [] })
  })

  it.each([
    { selected: [], label: 'All groups' },
    { selected: [10], label: '1 group selected' },
    { selected: [10, 20], label: '2 groups selected' },
  ])('labels the trigger $label', async ({ selected, label }) => {
    useUIStore.setState({ runsFeedGroupIds: selected })
    renderWithProviders(<GroupFilter />)
    expect(await screen.findByText(label)).toBeInTheDocument()
  })

  it('lists only parent projects (entries with children)', async () => {
    await openFilter()

    expect(screen.getByText('Acme')).toBeInTheDocument()
    expect(screen.getByText('globex')).toBeInTheDocument()
    expect(screen.queryByText('api-cloud')).not.toBeInTheDocument()
    expect(screen.queryByText('standalone')).not.toBeInTheDocument()
  })

  it.each([
    { selected: [], want: [10] },
    { selected: [10, 20], want: [20] },
  ])('toggling Acme turns runsFeedGroupIds $selected into $want', async ({ selected, want }) => {
    useUIStore.setState({ runsFeedGroupIds: selected })
    const user = await openFilter()
    await user.click(screen.getByText('Acme'))

    expect(useUIStore.getState().runsFeedGroupIds).toEqual(want)
  })
})
