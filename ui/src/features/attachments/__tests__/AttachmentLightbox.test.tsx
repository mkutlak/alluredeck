import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { AttachmentEntry } from '@/types/api'

import { AttachmentLightbox } from '../AttachmentLightbox'

const imageAttachment: AttachmentEntry = {
  id: 1, name: 'screenshot.png', source: 'abc.png', mime_type: 'image/png', size_bytes: 1024, url: '/mock/abc.png',
}

describe('AttachmentLightbox', () => {
  it('does not render when open=false', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={false} onOpenChange={() => {}} />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders image preview for image/* mime type', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('img')).toBeInTheDocument()
  })

  it('shows download button', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByRole('link', { name: /download/i })).toBeInTheDocument()
  })

  it('shows attachment name in title', () => {
    render(<AttachmentLightbox attachment={imageAttachment} open={true} onOpenChange={() => {}} />)
    expect(screen.getByText('screenshot.png')).toBeInTheDocument()
  })
})
