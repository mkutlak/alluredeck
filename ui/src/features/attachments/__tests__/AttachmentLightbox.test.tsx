import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AttachmentEntry } from '@/types/api'

import { AttachmentLightbox } from '../AttachmentLightbox'
import { downloadAttachment } from '@/api/attachments'

vi.mock('@/api/attachments', () => ({
  downloadAttachment: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../AttachmentTextPreview', () => ({
  AttachmentTextPreview: ({ fileName }: { fileName: string }) => (
    <div data-testid="text-preview">Preview: {fileName}</div>
  ),
}))

function attachment(name: string, mime_type: string): AttachmentEntry {
  return { id: 1, name, source: name, mime_type, size_bytes: 1024, url: `/mock/${name}` }
}

function renderLightbox(entry: AttachmentEntry) {
  render(<AttachmentLightbox attachment={entry} open={true} onOpenChange={() => {}} />)
}

describe('AttachmentLightbox', () => {
  it('renders image preview with crossOrigin for image/* mime type', () => {
    renderLightbox(attachment('screenshot.png', 'image/png'))
    expect(screen.getByRole('img')).toHaveAttribute('crossOrigin', 'use-credentials')
  })

  it.each([
    { name: 'stdout.txt', mime: 'text/plain' },
    { name: 'data.json', mime: 'application/json' },
  ])('renders the text preview for $mime attachments', ({ name, mime }) => {
    renderLightbox(attachment(name, mime))
    expect(screen.getByTestId('text-preview')).toHaveTextContent(`Preview: ${name}`)
  })

  // A button, not a link: cross-origin <a download> doesn't work.
  it('downloads through downloadAttachment from a button, not a link', async () => {
    renderLightbox(attachment('screenshot.png', 'image/png'))
    expect(screen.queryByRole('link', { name: /download/i })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /download/i }))
    expect(downloadAttachment).toHaveBeenCalledWith('/mock/screenshot.png', 'screenshot.png')
  })
})
