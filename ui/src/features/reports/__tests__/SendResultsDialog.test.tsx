import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
vi.mock('@/components/ui/use-toast', () => ({ toast: vi.fn() }))
mockApiClient()

import { generateReport, getJobStatus, sendResultsMultipart } from '@/api/reports'
import { toast } from '@/components/ui/use-toast'
import { SendResultsDialog } from '../SendResultsDialog'

// The numeric project id is an internal key; the caller passes the label the rest of the UI uses
// (a child reads parent/child).
const LABEL = 'webshop/webshop-web'

function renderDialog() {
  return renderWithProviders(
    <SendResultsDialog projectId={3} label={LABEL} open onOpenChange={vi.fn()} />,
  )
}

function dropResultFile() {
  fireEvent.drop(screen.getByRole('button', { name: /drop zone/i }), {
    dataTransfer: { files: [new File(['{}'], 'a-result.json')] },
  })
}

describe('SendResultsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(sendResultsMultipart).mockResolvedValue(undefined)
  })

  it('names the project by its label, not its numeric id', () => {
    renderDialog()

    expect(screen.getByText(LABEL)).toBeInTheDocument()
    expect(screen.queryByText('3')).not.toBeInTheDocument()
  })

  it('names the project by its label in the upload-only confirmation toast', async () => {
    renderDialog()

    dropResultFile()
    const user = userEvent.setup()
    await user.click(screen.getByRole('checkbox', { name: /generate report after upload/i }))
    await user.click(screen.getByRole('button', { name: /upload/i }))

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith({
        title: 'Results sent',
        description: `1 file uploaded to "${LABEL}".`,
      }),
    )
  })

  it('names the project by its label in the report-generated toast', async () => {
    vi.mocked(generateReport).mockResolvedValue({
      data: { job_id: 'job-1' },
      metadata: { message: 'ok' },
    })
    vi.mocked(getJobStatus).mockResolvedValue({
      data: {
        job_id: 'job-1',
        project_id: 3,
        slug: 'webshop-web',
        status: 'completed',
        created_at: '2026-03-01T10:00:00Z',
        started_at: null,
        completed_at: null,
        output: '',
        error: '',
      },
      metadata: { message: 'ok' },
    })
    renderDialog()

    dropResultFile()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /upload/i }))

    await waitFor(() =>
      expect(toast).toHaveBeenCalledWith({
        title: 'Report generated',
        description: `1 file uploaded and report generated for "${LABEL}".`,
      }),
    )
  })
})
