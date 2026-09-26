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

const EMPTY: AttachmentsData = { groups: [], total: 0, limit: 100, offset: 0 }

function renderTab(data: AttachmentsData = mockData) {
  vi.mocked(fetchAttachments).mockResolvedValue(data)
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/projects/proj1/attachments']}>
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

  it('renders each test group with status, file count, and attachments under the latest report', async () => {
    renderTab()

    expect(await screen.findByText('shouldRegisterNewUser')).toBeInTheDocument()
    expect(screen.getByText('shouldLogin')).toBeInTheDocument()
    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
    expect(screen.getByText('log.txt')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    expect(screen.getByText('passed')).toBeInTheDocument()
    expect(screen.getByText('2 files')).toBeInTheDocument()
    expect(screen.getByText('1 file')).toBeInTheDocument()
    expect(await screen.findByText(/Report #5 \(latest\)/)).toBeInTheDocument()
    // Status filter and report selector.
    expect(screen.getAllByRole('combobox')).toHaveLength(2)
  })

  it('collapses and expands groups on click', async () => {
    const user = userEvent.setup()
    renderTab()
    await screen.findByText('screenshot.png')

    await user.click(screen.getByText('shouldRegisterNewUser'))
    expect(screen.queryByText('screenshot.png')).not.toBeInTheDocument()

    await user.click(screen.getByText('shouldRegisterNewUser'))
    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
  })

  it('shows empty state when no attachments', async () => {
    renderTab(EMPTY)
    expect(await screen.findByText(/no attachments/i)).toBeInTheDocument()
  })

  // Wiring for filterAttachments (rules covered in utils.test.ts).
  it('filters by Images and updates the subtitle count', async () => {
    const user = userEvent.setup()
    renderTab()
    await screen.findByText('screenshot.png')

    await user.click(screen.getByRole('button', { name: /images/i }))

    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
    expect(screen.queryByText('stdout.txt')).not.toBeInTheDocument()
    expect(screen.queryByText('log.txt')).not.toBeInTheDocument()
    expect(screen.getByText(/1 of 3/)).toBeInTheDocument()
  })

  it('forwards the status filter, labels the subtitle, and uses status-aware empty copy', async () => {
    const user = userEvent.setup()
    renderTab()
    await screen.findByText('shouldRegisterNewUser')
    expect(fetchAttachments).toHaveBeenCalledWith('proj1', 'latest', undefined)

    vi.mocked(fetchAttachments).mockResolvedValue(EMPTY)
    // The status filter is the first combobox (left of the report selector).
    await user.click(screen.getAllByRole('combobox')[0]!)
    await user.click(await screen.findByRole('option', { name: /^failed$/i }))

    expect(fetchAttachments).toHaveBeenLastCalledWith('proj1', 'latest', { status: 'failed' })
    expect(await screen.findByText(/failed only/i)).toBeInTheDocument()
    expect(await screen.findByText(/status filter/i)).toBeInTheDocument()
  })
})
