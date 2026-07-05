import { useEffect, useState } from 'react'
import DOMPurify from 'dompurify'

import { cn } from '@/lib/utils'

// Prose Tailwind block shared with AttachmentTextPreview's markdown rendering.
const PROSE_CLASSES =
  '[&_code]:bg-muted [&_pre]:bg-muted [&_blockquote]:border-muted-foreground/30 [&_a]:text-primary text-sm [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:pl-3 [&_blockquote]:italic [&_code]:rounded [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_h1]:mb-3 [&_h1]:text-lg [&_h1]:font-bold [&_h2]:mb-2 [&_h2]:text-base [&_h2]:font-semibold [&_h3]:mb-2 [&_h3]:text-sm [&_h3]:font-semibold [&_li]:mb-1 [&_ol]:mb-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:mb-2 [&_pre]:mb-2 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:p-3 [&_ul]:mb-2 [&_ul]:list-disc [&_ul]:pl-5'

export interface MarkdownProps {
  text: string
  className?: string
}

/**
 * Renders markdown text as sanitized HTML using `marked` + DOMPurify.
 * Extracted from AttachmentTextPreview's inline recipe for reuse.
 */
export function Markdown({ text, className }: MarkdownProps) {
  const [html, setHtml] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    import('marked')
      .then(({ marked }) => marked(text))
      .then((rawHtml) => {
        if (cancelled) return
        setHtml(DOMPurify.sanitize(rawHtml, { USE_PROFILES: { html: true } }))
      })
      .catch(() => {
        if (!cancelled) setHtml(`<pre>${DOMPurify.sanitize(text)}</pre>`)
      })

    return () => {
      cancelled = true
    }
  }, [text])

  if (html == null) {
    return null
  }

  return (
    <div
      className={cn(PROSE_CLASSES, className)}
      data-testid="markdown-content"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
