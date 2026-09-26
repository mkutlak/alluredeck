import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AttachmentEntry } from '@/types/api'

import { AttachmentRow } from '../AttachmentRow'

function attachment(name: string, mime_type: string): AttachmentEntry {
  return { id: 1, name, source: name, mime_type, size_bytes: 1024, url: `/mock/${name}` }
}

/**
 * AttachmentRow uses `display: contents` to participate in a parent CSS grid,
 * so we wrap it in a matching grid container for proper rendering.
 */
function renderRow(entry: AttachmentEntry, onView = vi.fn()) {
  render(
    <div style={{ display: 'grid', gridTemplateColumns: '1.25rem 1fr auto auto auto' }}>
      <AttachmentRow attachment={entry} onView={onView} />
    </div>,
  )
  return onView
}

describe('AttachmentRow', () => {
  it.each([
    { name: 'screenshot.png', mime: 'image/png', badge: 'IMAGE' },
    { name: 'app.log', mime: 'text/plain', badge: 'LOG' },
    { name: 'trace-chromium.zip', mime: 'application/zip', badge: 'TRACE' },
    { name: 'recording.webm', mime: 'video/webm', badge: 'VIDEO' },
    { name: 'data.bin', mime: 'application/octet-stream', badge: 'OTHER' },
  ])('shows the $badge badge for $name ($mime)', ({ name, mime, badge }) => {
    renderRow(attachment(name, mime))
    expect(screen.getByText(badge)).toBeInTheDocument()
  })

  it.each([
    { trigger: 'the filename', target: () => screen.getByText('screenshot.png') },
    { trigger: 'the View button', target: () => screen.getByLabelText('View') },
  ])('calls onView once when $trigger is clicked', async ({ target }) => {
    const onView = renderRow(attachment('screenshot.png', 'image/png'))
    await userEvent.click(target())
    expect(onView).toHaveBeenCalledTimes(1)
  })

  it('links the download to attachment.url + "?dl=1" with a download attribute', () => {
    renderRow(attachment('screenshot.png', 'image/png'))
    const link = screen.getByLabelText('Download')
    expect(link).toHaveAttribute('href', '/mock/screenshot.png?dl=1')
    expect(link).toHaveAttribute('download')
  })
})
