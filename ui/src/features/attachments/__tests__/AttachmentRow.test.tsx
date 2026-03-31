import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AttachmentEntry } from '@/types/api'

import { AttachmentRow } from '../AttachmentRow'

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const imageAttachment: AttachmentEntry = {
  id: 1, name: 'screenshot.png', source: 'abc.png', mime_type: 'image/png', size_bytes: 1024, url: '/mock/abc.png',
}
const textAttachment: AttachmentEntry = {
  id: 2, name: 'app.log', source: 'def.txt', mime_type: 'text/plain', size_bytes: 2048, url: '/mock/def.txt',
}
const traceAttachment: AttachmentEntry = {
  id: 3, name: 'trace-chromium.zip', source: 'ghi.zip', mime_type: 'application/zip', size_bytes: 1048576, url: '/mock/ghi.zip',
}
const videoAttachment: AttachmentEntry = {
  id: 4, name: 'recording.webm', source: 'jkl.webm', mime_type: 'video/webm', size_bytes: 3145728, url: '/mock/jkl.webm',
}
const otherAttachment: AttachmentEntry = {
  id: 5, name: 'data.bin', source: 'mno.bin', mime_type: 'application/octet-stream', size_bytes: 512, url: '/mock/mno.bin',
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * AttachmentRow uses `display: contents` to participate in a parent CSS grid,
 * so we wrap it in a matching grid container for proper rendering.
 */
function renderRow(attachment: AttachmentEntry, onView = vi.fn()) {
  return render(
    <div style={{ display: 'grid', gridTemplateColumns: '1.25rem 1fr auto auto auto' }}>
      <AttachmentRow attachment={attachment} onView={onView} />
    </div>,
  )
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('AttachmentRow', () => {
  describe('MIME badge', () => {
    it('shows IMAGE badge for image/png attachment', () => {
      renderRow(imageAttachment)
      expect(screen.getByText('IMAGE')).toBeInTheDocument()
    })

    it('shows LOG badge for text/plain attachment', () => {
      renderRow(textAttachment)
      expect(screen.getByText('LOG')).toBeInTheDocument()
    })

    it('shows TRACE badge for application/zip attachment with trace name pattern', () => {
      renderRow(traceAttachment)
      expect(screen.getByText('TRACE')).toBeInTheDocument()
    })

    it('shows VIDEO badge for video/webm attachment', () => {
      renderRow(videoAttachment)
      expect(screen.getByText('VIDEO')).toBeInTheDocument()
    })

    it('shows OTHER badge for application/octet-stream attachment', () => {
      renderRow(otherAttachment)
      expect(screen.getByText('OTHER')).toBeInTheDocument()
    })
  })

  describe('filename and size', () => {
    it('renders the attachment filename', () => {
      renderRow(imageAttachment)
      expect(screen.getByText('screenshot.png')).toBeInTheDocument()
    })

    it('renders formatted size for 1024 bytes as "1 KB"', () => {
      renderRow(imageAttachment)
      expect(screen.getByText('1 KB')).toBeInTheDocument()
    })

    it('renders formatted size for 2048 bytes as "2 KB"', () => {
      renderRow(textAttachment)
      expect(screen.getByText('2 KB')).toBeInTheDocument()
    })

    it('renders formatted size for 1048576 bytes as "1 MB"', () => {
      renderRow(traceAttachment)
      expect(screen.getByText('1 MB')).toBeInTheDocument()
    })
  })

  describe('filename title tooltip', () => {
    it('has title attribute equal to the full filename on the filename element', () => {
      renderRow(imageAttachment)
      expect(screen.getByTitle('screenshot.png')).toBeInTheDocument()
    })

    it('has title attribute equal to the full filename for text attachment', () => {
      renderRow(textAttachment)
      expect(screen.getByTitle('app.log')).toBeInTheDocument()
    })
  })

  describe('onView callback — filename click', () => {
    it('calls onView when the filename button is clicked', async () => {
      const onView = vi.fn()
      renderRow(imageAttachment, onView)
      await userEvent.click(screen.getByText('screenshot.png'))
      expect(onView).toHaveBeenCalledTimes(1)
    })

    it('does not call onView before any interaction', () => {
      const onView = vi.fn()
      renderRow(imageAttachment, onView)
      expect(onView).not.toHaveBeenCalled()
    })
  })

  describe('onView callback — view button', () => {
    it('calls onView when the view button is clicked', async () => {
      const onView = vi.fn()
      renderRow(imageAttachment, onView)
      await userEvent.click(screen.getByLabelText('View'))
      expect(onView).toHaveBeenCalledTimes(1)
    })

    it('renders a button with aria-label "View"', () => {
      renderRow(imageAttachment)
      expect(screen.getByLabelText('View')).toBeInTheDocument()
    })
  })

  describe('download link', () => {
    it('renders a link with aria-label "Download"', () => {
      renderRow(imageAttachment)
      expect(screen.getByLabelText('Download')).toBeInTheDocument()
    })

    it('download link href equals attachment.url + "?dl=1"', () => {
      renderRow(imageAttachment)
      expect(screen.getByLabelText('Download')).toHaveAttribute('href', '/mock/abc.png?dl=1')
    })

    it('download link has download attribute', () => {
      renderRow(imageAttachment)
      expect(screen.getByLabelText('Download')).toHaveAttribute('download')
    })

    it('download link href is correct for text attachment', () => {
      renderRow(textAttachment)
      expect(screen.getByLabelText('Download')).toHaveAttribute('href', '/mock/def.txt?dl=1')
    })
  })
})
