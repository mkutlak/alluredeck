import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { Markdown } from '../Markdown'

vi.mock('dompurify', () => ({
  default: {
    sanitize: vi.fn((html: string) => html),
  },
}))

describe('Markdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing while markdown is being parsed', () => {
    const { container } = render(<Markdown text="# Heading" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders parsed and sanitized markdown as HTML', async () => {
    render(<Markdown text="**bold hypothesis**" />)

    await waitFor(() => {
      expect(screen.getByTestId('markdown-content')).toBeInTheDocument()
    })
    expect(screen.getByTestId('markdown-content').innerHTML).toContain('<strong>bold hypothesis')
  })

  it('merges a custom className with the prose classes', async () => {
    render(<Markdown text="plain text" className="custom-class" />)

    await waitFor(() => {
      expect(screen.getByTestId('markdown-content')).toBeInTheDocument()
    })
    expect(screen.getByTestId('markdown-content').className).toContain('custom-class')
  })
})
