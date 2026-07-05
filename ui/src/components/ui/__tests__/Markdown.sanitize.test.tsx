import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { Markdown } from '../Markdown'

// Deliberately does NOT mock 'dompurify' or 'marked' — unlike Markdown.test.tsx
// (which mocks DOMPurify to an identity function for behavioral assertions),
// this file exercises the real sanitization path end-to-end so the
// security-relevant stripping behavior is actually covered.
describe('Markdown (real DOMPurify sanitization)', () => {
  it('strips an onerror attribute from an injected <img> tag', async () => {
    render(<Markdown text='<img src="x" onerror="alert(1)">' />)

    await waitFor(() => {
      expect(screen.getByTestId('markdown-content')).toBeInTheDocument()
    })
    const html = screen.getByTestId('markdown-content').innerHTML
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('alert(1)')
  })

  it('removes a <script> tag entirely while keeping surrounding safe text', async () => {
    render(<Markdown text="<script>alert(1)</script>Safe text" />)

    await waitFor(() => {
      expect(screen.getByTestId('markdown-content')).toBeInTheDocument()
    })
    const html = screen.getByTestId('markdown-content').innerHTML
    expect(html).not.toContain('<script')
    expect(html).not.toContain('alert(1)')
    expect(html).toContain('Safe text')
  })
})
