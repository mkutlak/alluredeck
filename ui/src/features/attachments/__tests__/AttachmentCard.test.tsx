import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AttachmentEntry } from '@/types/api'

import { AttachmentCard } from '../AttachmentCard'

const imageAttachment: AttachmentEntry = {
  id: 1, name: 'screenshot.png', source: 'abc.png', mime_type: 'image/png', size_bytes: 1024, url: '/mock/abc.png',
}

const textAttachment: AttachmentEntry = {
  id: 2, name: 'app.log', source: 'def.txt', mime_type: 'text/plain', size_bytes: 2048, url: '/mock/def.txt',
}

describe('AttachmentCard', () => {
  it('renders thumbnail for image attachments', () => {
    render(<AttachmentCard attachment={imageAttachment} onClick={() => {}} />)
    const img = screen.getByRole('img')
    expect(img).toBeInTheDocument()
  })

  it('renders file icon for text attachments', () => {
    render(<AttachmentCard attachment={textAttachment} onClick={() => {}} />)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByText('app.log')).toBeInTheDocument()
  })

  it('calls onClick when clicked', async () => {
    const onClick = vi.fn()
    render(<AttachmentCard attachment={imageAttachment} onClick={onClick} />)
    await userEvent.click(screen.getByRole('button'))
    expect(onClick).toHaveBeenCalled()
  })
})
