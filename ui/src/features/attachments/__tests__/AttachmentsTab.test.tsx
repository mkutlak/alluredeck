import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import type { AttachmentsData } from '@/types/api'

vi.mock('@/api/attachments', () => ({
  fetchAttachments: vi.fn(),
  attachmentFileUrl: vi.fn((_pid: string, _rid: string, source: string) => `/mock/${source}`),
  downloadAttachment: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/api/reports', () => ({
  fetchReportHistory: vi.fn().mockResolvedValue({
    data: {
      reports: [
        {
          report_id: '5',
          is_latest: true,
          generated_at: '2026-03-29T15:00:00Z',
          statistic: null,
          duration_ms: null,
        },
        {
          report_id: '4',
          is_latest: false,
          generated_at: '2026-03-28T15:00:00Z',
          statistic: null,
          duration_ms: null,
        },
      ],
    },
    metadata: { page: 1, per_page: 50, total_items: 2, total_pages: 1 },
  }),
}))

import { fetchAttachments } from '@/api/attachments'
import { AttachmentsTab } from '../AttachmentsTab'

function renderTab(projectId = 'proj1') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={[`/projects/${projectId}/attachments`]}>
        <Routes>
          <Route path="projects/:id/attachments" element={<AttachmentsTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const mockData: AttachmentsData = {
  groups: [
    {
      test_name: 'shouldRegisterNewUser',
      test_status: 'failed',
      attachments: [
        {
          id: 1,
          name: 'screenshot.png',
          source: 'abc123.png',
          mime_type: 'image/png',
          size_bytes: 1024,
          url: '/mock/abc123.png',
        },
        {
          id: 2,
          name: 'stdout.txt',
          source: 'def456.txt',
          mime_type: 'text/plain',
          size_bytes: 369,
          url: '/mock/def456.txt',
        },
      ],
    },
    {
      test_name: 'shouldLogin',
      test_status: 'passed',
      attachments: [
        {
          id: 3,
          name: 'log.txt',
          source: 'ghi789.txt',
          mime_type: 'text/plain',
          size_bytes: 2048,
          url: '/mock/ghi789.txt',
        },
      ],
    },
  ],
  total: 3,
  limit: 100,
  offset: 0,
}

describe('AttachmentsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons while fetching', () => {
    vi.mocked(fetchAttachments).mockReturnValue(new Promise(() => {}))
    renderTab()
    const skeletons = document.querySelectorAll('[class*="animate-pulse"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders grouped attachments with test names', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    expect(await screen.findByText('shouldRegisterNewUser')).toBeInTheDocument()
    expect(screen.getByText('shouldLogin')).toBeInTheDocument()
    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
    expect(screen.getByText('log.txt')).toBeInTheDocument()
  })

  it('shows test status for each group', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    await screen.findByText('shouldRegisterNewUser')
    expect(screen.getByText('failed')).toBeInTheDocument()
    expect(screen.getByText('passed')).toBeInTheDocument()
  })

  it('shows file count per group', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    await screen.findByText('shouldRegisterNewUser')
    expect(screen.getByText('2 files')).toBeInTheDocument()
    expect(screen.getByText('1 file')).toBeInTheDocument()
  })

  it('collapses and expands groups on click', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    await screen.findByText('screenshot.png')

    // Click group header to collapse
    await userEvent.click(screen.getByText('shouldRegisterNewUser'))
    expect(screen.queryByText('screenshot.png')).not.toBeInTheDocument()

    // Click again to expand
    await userEvent.click(screen.getByText('shouldRegisterNewUser'))
    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
  })

  it('shows empty state when no attachments', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue({
      groups: [],
      total: 0,
      limit: 100,
      offset: 0,
    })
    renderTab()
    expect(await screen.findByText(/no attachments/i)).toBeInTheDocument()
  })

  it('renders MIME filter buttons', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    await screen.findByText('shouldRegisterNewUser')
    expect(screen.getByRole('button', { name: /all/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /images/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /logs/i })).toBeInTheDocument()
  })

  it('shows report number and report selector', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    await screen.findByText('shouldRegisterNewUser')
    // Should show report label with build number
    expect(screen.getByText(/Report #5 \(latest\)/)).toBeInTheDocument()
    // Should have report selector
    expect(screen.getByRole('combobox')).toBeInTheDocument()
  })
})
