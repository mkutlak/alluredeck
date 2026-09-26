import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/api/attachments', () => ({
  fetchAttachmentContent: vi.fn(),
}))

// Unescaped on purpose: DOMPurify (real, not mocked) must neutralise what shiki emits.
vi.mock('shiki', () => ({
  createHighlighter: vi.fn().mockResolvedValue({
    codeToHtml: vi.fn((code: string) => `<pre class="shiki"><code>${code}</code></pre>`),
  }),
}))

import { fetchAttachmentContent } from '@/api/attachments'
import { AttachmentTextPreview } from '../AttachmentTextPreview'

function renderPreview(content: string | Error) {
  if (content instanceof Error) vi.mocked(fetchAttachmentContent).mockRejectedValue(content)
  else vi.mocked(fetchAttachmentContent).mockResolvedValue(content)
  render(<AttachmentTextPreview url="/mock/file.txt" mimeType="text/plain" fileName="stdout.txt" />)
}

describe('AttachmentTextPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a skeleton, then sanitized highlighted content, and copies the raw text', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    const raw = 'Hello World\n<img src="x" onerror="alert(1)">'
    renderPreview(raw)

    expect(screen.getByTestId('text-preview-loading')).toBeInTheDocument()
    const content = await screen.findByTestId('text-preview-content')
    expect(content.innerHTML).toContain('Hello World')
    expect(content.innerHTML).not.toContain('onerror')

    await userEvent.click(screen.getByRole('button', { name: /copy/i }))
    expect(writeText).toHaveBeenCalledWith(raw)
  })

  it('shows error state on fetch failure', async () => {
    renderPreview(new Error('Network error'))
    expect(await screen.findByText('Network error')).toBeInTheDocument()
  })

  it('shows truncation warning for large files', async () => {
    renderPreview('x'.repeat(600_000))
    await screen.findByTestId('text-preview-content')
    expect(screen.getByText(/truncated/i)).toBeInTheDocument()
  })
})
