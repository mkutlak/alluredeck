import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { MemoryRouter, Routes, Route } from 'react-router'
import { AdminPage } from '../AdminPage'
import * as adminApi from '@/api/admin'
import { useAuthStore, type Role } from '@/store/auth'
import type { AdminJobEntry, AdminResultsEntry } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/admin')
mockApiClient()

function renderPage({
  jobs = [],
  results = [],
  roles = ['admin'],
}: { jobs?: AdminJobEntry[]; results?: AdminResultsEntry[]; roles?: Role[] } = {}) {
  useAuthStore.setState({ roles })
  vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue({
    data: jobs,
    metadata: { message: 'OK' },
    pagination: { page: 1, per_page: 20, total: jobs.length, total_pages: 1 },
  })
  vi.mocked(adminApi.fetchAdminResults).mockResolvedValue(results)
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/admin']}>
        <Routes>
          <Route path="/admin" element={<AdminPage />} />
          <Route path="/" element={<div data-testid="dashboard" />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function makeJob(overrides: Partial<AdminJobEntry> = {}): AdminJobEntry {
  return {
    job_id: 'job-123',
    project_id: 1,
    slug: 'my-project',
    status: 'running',
    created_at: '2026-03-07T10:00:00Z',
    started_at: '2026-03-07T10:00:01Z',
    completed_at: null,
    output: '',
    error: '',
    ...overrides,
  }
}

const done = (job_id: string, status: 'completed' | 'failed' = 'completed') =>
  makeJob({ job_id, status, completed_at: '2026-03-07T10:01:00Z' })

describe('AdminPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it.each([
    { roles: [] as Role[], admin: false },
    { roles: ['admin'] as Role[], admin: true },
  ])('shows System Monitor only to admins (roles $roles)', ({ roles, admin }) => {
    renderPage({ roles })
    expect(screen.queryByText('System Monitor') !== null).toBe(admin)
    expect(screen.queryByTestId('dashboard') !== null).toBe(!admin)
  })

  it('shows empty states when there are no jobs and no pending results', async () => {
    renderPage()
    expect(await screen.findByText(/no jobs/i)).toBeInTheDocument()
    expect(await screen.findByText(/no unprocessed results/i)).toBeInTheDocument()
  })

  it('offers Cancel only for active jobs and selection only for terminal jobs', async () => {
    vi.mocked(adminApi.cancelJob).mockResolvedValue()
    renderPage({
      jobs: [
        makeJob({ job_id: 'job-abc', slug: 'proj-alpha', status: 'running' }),
        done('job-done'),
        done('job-failed', 'failed'),
      ],
    })

    expect(await screen.findByText('proj-alpha')).toBeInTheDocument()
    // select-all in header + 2 terminal job rows = 3 checkboxes total
    expect(screen.getAllByRole('checkbox')).toHaveLength(3)

    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    await waitFor(() => {
      // TanStack Query v5 passes (variables, context) to mutation fn
      expect(adminApi.cancelJob).toHaveBeenCalledWith('job-abc', expect.anything())
    })
  })

  it('selects terminal jobs singly or all at once, then deletes them after confirmation', async () => {
    vi.mocked(adminApi.deleteJob).mockResolvedValue()
    renderPage({
      jobs: [makeJob({ job_id: 'job-running' }), done('job-done-1'), done('job-done-2', 'failed')],
    })

    const rowBox = await screen.findByRole('checkbox', { name: 'Select job job-done-1' })
    expect(screen.queryByRole('button', { name: /delete selected/i })).not.toBeInTheDocument()

    await userEvent.click(rowBox)
    expect(screen.getByRole('button', { name: /delete selected \(1\)/i })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('checkbox', { name: /select all terminal jobs/i }))
    await userEvent.click(screen.getByRole('button', { name: /delete selected \(2\)/i }))
    await userEvent.click(await screen.findByRole('button', { name: /^confirm$/i }))

    await waitFor(() => {
      expect(adminApi.deleteJob).toHaveBeenCalledTimes(2)
    })
    expect(adminApi.deleteJob).toHaveBeenCalledWith('job-done-1')
    expect(adminApi.deleteJob).toHaveBeenCalledWith('job-done-2')
  })

  it('delete button triggers confirmation dialog and calls API on confirm', async () => {
    vi.mocked(adminApi.cleanAdminResults).mockResolvedValue()
    renderPage({
      results: [
        {
          project_id: 3,
          slug: 'proj-del',
          storage_key: 'proj-del',
          file_count: 5,
          total_size: 1048576,
          last_modified: '2026-03-07T09:00:00Z',
        },
      ],
    })

    await userEvent.click(await screen.findByRole('button', { name: /^delete$/i }))
    // Exact name, so the Delete trigger does not match.
    await userEvent.click(await screen.findByRole('button', { name: /^confirm$/i }))

    await waitFor(() => {
      expect(adminApi.cleanAdminResults).toHaveBeenCalledWith('proj-del', expect.anything())
    })
  })
})
