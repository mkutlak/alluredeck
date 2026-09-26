import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Markdown } from '../Markdown'

// Deliberately does NOT mock 'dompurify' or 'marked': the real sanitization
// path runs end-to-end so the security-relevant stripping is actually covered.
describe('Markdown (real DOMPurify sanitization)', () => {
  it('renders nothing while markdown is being parsed', () => {
    const { container } = render(<Markdown text="# Heading" />)
    expect(container).toBeEmptyDOMElement()
  })

  it.each([
    [
      'strips an onerror attribute from an injected <img> tag',
      '<img src="x" onerror="alert(1)">',
      ['onerror', 'alert(1)'],
      '<img src="x">',
    ],
    [
      'removes a <script> tag entirely while keeping surrounding safe text',
      '<script>alert(1)</script>Safe text',
      ['<script', 'alert(1)'],
      'Safe text',
    ],
    [
      'keeps parsed markdown formatting',
      '**bold hypothesis**',
      [],
      '<strong>bold hypothesis</strong>',
    ],
  ])('%s', async (_, text, forbidden, kept) => {
    render(<Markdown text={text} />)

    const html = (await screen.findByTestId('markdown-content')).innerHTML
    for (const fragment of forbidden) expect(html).not.toContain(fragment)
    expect(html).toContain(kept)
  })
})
