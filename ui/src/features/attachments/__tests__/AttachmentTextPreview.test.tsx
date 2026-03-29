import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/api/attachments', () => ({
  fetchAttachmentContent: vi.fn(),
}))

vi.mock('shiki', () => ({
  createHighlighter: vi.fn().mockResolvedValue({
    codeToHtml: vi.fn(
      (code: string) => `<pre class="shiki"><code>${code}</code></pre>`,
    ),
  }),
}))

vi.mock('dompurify', () => ({
  default: {
    sanitize: vi.fn((html: string) => html),
  },
}))

import { fetchAttachmentContent } from '@/api/attachments'
import { AttachmentTextPreview } from '../AttachmentTextPreview'

describe('AttachmentTextPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeleton while fetching', () => {
    vi.mocked(fetchAttachmentContent).mockReturnValue(new Promise(() => {}))
    render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)
    expect(screen.getByTestId('text-preview-loading')).toBeInTheDocument()
  })

  it('renders fetched text content', async () => {
    vi.mocked(fetchAttachmentContent).mockResolvedValue('Hello World\nLine 2')
    render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)

    await waitFor(() => {
      expect(screen.getByTestId('text-preview-content')).toBeInTheDocument()
    })
    expect(screen.getByTestId('text-preview-content').innerHTML).toContain('Hello World')
  })

  it('shows error state on fetch failure', async () => {
    vi.mocked(fetchAttachmentContent).mockRejectedValue(new Error('Network error'))
    render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)

    expect(await screen.findByText('Network error')).toBeInTheDocument()
  })

  it('shows copy button', async () => {
    vi.mocked(fetchAttachmentContent).mockResolvedValue('some content')
    render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)

    await waitFor(() => {
      expect(screen.getByTestId('text-preview-content')).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument()
  })

  it('shows truncation warning for large files', async () => {
    const largeContent = 'x'.repeat(600_000)
    vi.mocked(fetchAttachmentContent).mockResolvedValue(largeContent)
    render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)

    await waitFor(() => {
      expect(screen.getByTestId('text-preview-content')).toBeInTheDocument()
    })
    expect(screen.getByText(/truncated/i)).toBeInTheDocument()
  })

  it('copies text to clipboard on button click', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    vi.mocked(fetchAttachmentContent).mockResolvedValue('copy me')
    render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)

    await waitFor(() => {
      expect(screen.getByTestId('text-preview-content')).toBeInTheDocument()
    })
    await userEvent.click(screen.getByRole('button', { name: /copy/i }))
    expect(writeText).toHaveBeenCalledWith('copy me')
  })
})
