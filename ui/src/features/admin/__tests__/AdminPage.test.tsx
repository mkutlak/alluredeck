import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import { MemoryRouter, Routes, Route } from 'react-router'
import { AdminPage } from '../AdminPage'
import * as adminApi from '@/api/admin'
import { useAuthStore } from '@/store/auth'
import type { AdminJobEntry, AdminResultsEntry } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(),
  selectIsAdmin: (s: { roles?: string[] }) => (s.roles ?? []).includes('admin'),
}))
vi.mock('@/api/admin')
mockApiClient()

import type { AuthState } from '@/store/auth'

type AuthSelector = (s: Partial<AuthState>) => unknown

function renderPage(initialPath = '/admin') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={[initialPath]}>
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

function makeResult(overrides: Partial<AdminResultsEntry> = {}): AdminResultsEntry {
  return {
    project_id: 1,
    slug: 'my-project',
    file_count: 5,
    total_size: 1048576, // 1 MB
    last_modified: '2026-03-07T09:00:00Z',
    ...overrides,
  }
}

describe('AdminPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: ['admin'] }),
    )
  })

  it('redirects non-admin to dashboard', () => {
    vi.mocked(useAuthStore).mockImplementation((selector: unknown) =>
      (selector as AuthSelector)({ roles: [] }),
    )
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    expect(screen.getByTestId('dashboard')).toBeInTheDocument()
    expect(screen.queryByText('System Monitor')).not.toBeInTheDocument()
  })

  it('renders page title for admin', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    expect(screen.getByText('System Monitor')).toBeInTheDocument()
  })

  it('shows empty state when no jobs', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/no jobs/i)).toBeInTheDocument()
    })
  })

  it('shows empty state when no pending results', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/no unprocessed results/i)).toBeInTheDocument()
    })
  })

  it('renders jobs table with job data', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ project_id: 2, slug: 'proj-alpha', status: 'running' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('proj-alpha')).toBeInTheDocument()
      expect(screen.getByText('running')).toBeInTheDocument()
    })
  })

  it('renders results table with file count and size', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([
      makeResult({ project_id: 2, slug: 'proj-beta', file_count: 3, total_size: 2048 }),
    ])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('proj-beta')).toBeInTheDocument()
      expect(screen.getByText('3')).toBeInTheDocument()
      expect(screen.getByText('2 KB')).toBeInTheDocument()
    })
  })

  it('cancel button calls cancelJob API for active jobs', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ job_id: 'job-abc', status: 'running' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])
    vi.mocked(adminApi.cancelJob).mockResolvedValue()

    renderPage()

    const cancelBtn = await screen.findByRole('button', { name: /cancel/i })
    await userEvent.click(cancelBtn)

    await waitFor(() => {
      // TanStack Query v5 passes (variables, context) to mutation fn
      expect(adminApi.cancelJob).toHaveBeenCalledWith('job-abc', expect.anything())
    })
  })

  it('does not show cancel button for completed jobs', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ status: 'completed', completed_at: '2026-03-07T10:01:00Z' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('completed')).toBeInTheDocument()
    })

    expect(screen.queryByRole('button', { name: /cancel/i })).not.toBeInTheDocument()
  })

  it('shows checkboxes only for terminal jobs', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ job_id: 'job-running', status: 'running' }),
      makeJob({ job_id: 'job-done', status: 'completed', completed_at: '2026-03-07T10:01:00Z' }),
      makeJob({ job_id: 'job-failed', status: 'failed', completed_at: '2026-03-07T10:01:00Z' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('running')).toBeInTheDocument()
    })

    // select-all in header + 2 terminal job rows = 3 checkboxes total
    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes).toHaveLength(3)
  })

  it('shows delete selected button when a terminal job is selected', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ job_id: 'job-done', status: 'completed', completed_at: '2026-03-07T10:01:00Z' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('completed')).toBeInTheDocument()
    })

    // no delete button initially
    expect(screen.queryByRole('button', { name: /delete selected/i })).not.toBeInTheDocument()

    // select the job row checkbox (index 1; index 0 is select-all)
    const checkboxes = screen.getAllByRole('checkbox')
    await userEvent.click(checkboxes[1])

    expect(screen.getByRole('button', { name: /delete selected/i })).toBeInTheDocument()
  })

  it('select all checkbox selects all terminal jobs', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ job_id: 'job-running', status: 'running' }),
      makeJob({ job_id: 'job-done-1', status: 'completed', completed_at: '2026-03-07T10:01:00Z' }),
      makeJob({ job_id: 'job-done-2', status: 'failed', completed_at: '2026-03-07T10:01:00Z' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('running')).toBeInTheDocument()
    })

    // click select-all (first checkbox)
    const checkboxes = screen.getAllByRole('checkbox')
    await userEvent.click(checkboxes[0])

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete selected \(2\)/i })).toBeInTheDocument()
    })
  })

  it('delete selected calls deleteJob for selected jobs after confirmation', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([
      makeJob({ job_id: 'job-done', status: 'completed', completed_at: '2026-03-07T10:01:00Z' }),
    ])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([])
    vi.mocked(adminApi.deleteJob).mockResolvedValue()

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('completed')).toBeInTheDocument()
    })

    // select the job
    const checkboxes = screen.getAllByRole('checkbox')
    await userEvent.click(checkboxes[1])

    // click "Delete selected (1)"
    const deleteSelectedBtn = screen.getByRole('button', { name: /delete selected/i })
    await userEvent.click(deleteSelectedBtn)

    // confirm in the dialog
    const confirmBtn = await screen.findByRole('button', { name: /^confirm$/i })
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(adminApi.deleteJob).toHaveBeenCalledWith('job-done')
    })
  })

  it('delete button triggers confirmation dialog and calls API on confirm', async () => {
    vi.mocked(adminApi.fetchAdminJobs).mockResolvedValue([])
    vi.mocked(adminApi.fetchAdminResults).mockResolvedValue([
      makeResult({ project_id: 3, slug: 'proj-del' }),
    ])
    vi.mocked(adminApi.cleanAdminResults).mockResolvedValue()

    renderPage()

    const deleteBtn = await screen.findByRole('button', { name: /^delete$/i })
    await userEvent.click(deleteBtn)

    // Confirm button appears in the dialog (exact name to avoid matching the Delete trigger)
    const confirmBtn = await screen.findByRole('button', { name: /^confirm$/i })
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(adminApi.cleanAdminResults).toHaveBeenCalledWith('proj-del', expect.anything())
    })
  })
})
