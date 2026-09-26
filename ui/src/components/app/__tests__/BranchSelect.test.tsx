import { describe, it, expect, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { useUIStore } from '@/store/ui'
import * as branchesApi from '@/api/branches'
import { BranchSelect } from '../BranchSelect'

vi.mock('@/api/branches')

describe('BranchSelect', () => {
  it('shows the stored selectedBranch and writes a new selection back to the UI store', async () => {
    const user = userEvent.setup()
    vi.mocked(branchesApi.fetchBranches).mockResolvedValue([
      { id: 1, project_id: 1, name: 'main', is_default: false, created_at: '2024-01-01T00:00:00Z' },
      { id: 2, project_id: 1, name: 'dev', is_default: false, created_at: '2024-01-01T00:00:00Z' },
    ])
    useUIStore.setState({ selectedBranch: 'main' })
    renderWithProviders(
      <Routes>
        <Route path="/projects/:id" element={<BranchSelect />} />
      </Routes>,
      { route: '/projects/1' },
    )

    await waitFor(() => expect(screen.getByRole('combobox')).toHaveTextContent('main'))
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: 'dev' }))

    expect(useUIStore.getState().selectedBranch).toBe('dev')
  })
})
