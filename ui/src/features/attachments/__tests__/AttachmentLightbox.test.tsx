import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AttachmentEntry } from '@/types/api'

import { AttachmentLightbox } from '../AttachmentLightbox'

vi.mock('@/api/attachments', () => ({
  downloadAttachment: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('../AttachmentTextPreview', () => ({
  AttachmentTextPreview: ({ fileName }: { fileName: string }) => (
    <div data-testid="text-preview">Preview: {fileName}</div>
  ),
}))

const imageAttachment: AttachmentEntry = {
  id: 1, name: 'screenshot.png', source: 'abc.png', mime_type: 'image/png', size_bytes: 1024, url: '/mock/abc.png',
}

describe('AttachmentLightbox', () => {
  it('does not render when open=false', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={false} onOpenChange={() => {}} />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders image preview with crossOrigin for image/* mime type', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    const img = screen.getByRole('img')
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('crossOrigin', 'use-credentials')
  })

  it('shows download button (not a link)', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    const btn = screen.getByRole('button', { name: /download/i })
    expect(btn).toBeInTheDocument()
    // Should NOT be a link — cross-origin <a download> doesn't work
    expect(screen.queryByRole('link', { name: /download/i })).not.toBeInTheDocument()
  })

  it('calls downloadAttachment on click', async () => {
    const { downloadAttachment } = await import('@/api/attachments')
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /download/i }))
    expect(downloadAttachment).toHaveBeenCalledWith('/mock/abc.png', 'screenshot.png')
  })

  it('shows attachment name in title', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
  })

  it('renders text preview for text attachments', () => {
    const textAttachment: AttachmentEntry = {
      id: 2, name: 'stdout.txt', source: 'abc.txt', mime_type: 'text/plain', size_bytes: 369, url: '/mock/abc.txt',
    }
    render(<AttachmentLightbox attachment={textAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByTestId('text-preview')).toBeInTheDocument()
    expect(screen.getByText('Preview: stdout.txt')).toBeInTheDocument()
  })

  it('renders text preview for JSON attachments', () => {
    const jsonAttachment: AttachmentEntry = {
      id: 3, name: 'data.json', source: 'def.json', mime_type: 'application/json', size_bytes: 128, url: '/mock/def.json',
    }
    render(<AttachmentLightbox attachment={jsonAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByTestId('text-preview')).toBeInTheDocument()
  })
})
