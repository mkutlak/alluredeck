import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import type { AttachmentsData } from '@/types/api'

vi.mock('@/api/attachments', () => ({
  fetchAttachments: vi.fn(),
  attachmentFileUrl: vi.fn((_pid: string, _rid: string, source: string) => `/mock/${source}`),
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
  attachments: [
    { id: 1, name: 'screenshot.png', source: 'abc123.png', mime_type: 'image/png', size_bytes: 1024, url: '/api/v1/projects/proj1/reports/1/attachments/abc123.png' },
    { id: 2, name: 'log.txt', source: 'def456.txt', mime_type: 'text/plain', size_bytes: 2048, url: '/api/v1/projects/proj1/reports/1/attachments/def456.txt' },
  ],
  total: 2,
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

  it('renders attachment cards when data available', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue(mockData)
    renderTab()
    expect(await screen.findByText('screenshot.png')).toBeInTheDocument()
    expect(screen.getByText('log.txt')).toBeInTheDocument()
  })

  it('shows empty state when no attachments', async () => {
    vi.mocked(fetchAttachments).mockResolvedValue({
      attachments: [],
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
    await screen.findByText('screenshot.png')
    expect(screen.getByRole('button', { name: /all/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /images/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /logs/i })).toBeInTheDocument()
  })
})
