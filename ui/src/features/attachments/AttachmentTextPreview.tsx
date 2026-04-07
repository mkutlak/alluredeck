import { useEffect, useState, useRef } from 'react'
import { useTheme } from 'next-themes'
import { Copy, Check, AlertTriangle } from 'lucide-react'
import { createHighlighter, type Highlighter } from 'shiki'
import DOMPurify from 'dompurify'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { fetchAttachmentContent } from '@/api/attachments'

const MAX_PREVIEW_BYTES = 500_000 // 500 KB

// Singleton highlighter — created once, reused across renders.
let highlighterPromise: Promise<Highlighter> | null = null

function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: ['github-dark', 'github-light'],
      langs: ['json', 'xml', 'java', 'python', 'javascript', 'log'],
    })
  }
  return highlighterPromise
}

function isMarkdown(mimeType: string, fileName: string): boolean {
  if (mimeType === 'text/markdown' || mimeType === 'text/x-markdown') return true
  const lower = fileName.toLowerCase()
  return lower.endsWith('.md') || lower.endsWith('.markdown')
}

function detectLanguage(mimeType: string, fileName: string): string {
  if (mimeType === 'application/json') return 'json'
  if (mimeType === 'application/xml' || mimeType === 'text/xml') return 'xml'
  if (mimeType === 'text/javascript' || mimeType === 'application/javascript') return 'javascript'

  const lower = fileName.toLowerCase()
  if (lower.includes('trace') || lower.includes('stacktrace')) return 'java'
  if (lower.endsWith('.json')) return 'json'
  if (lower.endsWith('.xml')) return 'xml'
  if (lower.endsWith('.py')) return 'python'
  if (lower.endsWith('.js')) return 'javascript'
  if (lower.endsWith('.java')) return 'java'

  return 'log'
}

interface AttachmentTextPreviewProps {
  url: string
  mimeType: string
  fileName: string
}

export function AttachmentTextPreview({ url, mimeType, fileName }: AttachmentTextPreviewProps) {
  const { resolvedTheme } = useTheme()
  const [rawText, setRawText] = useState<string | null>(null)
  const [html, setHtml] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [truncated, setTruncated] = useState(false)
  const [copied, setCopied] = useState(false)
  const [isMarkdownContent, setIsMarkdownContent] = useState(false)
  const copyTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Reset state when the URL changes (React-recommended render-time pattern).
  const [prevUrl, setPrevUrl] = useState(url)
  if (url !== prevUrl) {
    setPrevUrl(url)
    setRawText(null)
    setHtml(null)
    setError(null)
    setTruncated(false)
    setIsMarkdownContent(false)
  }

  // Fetch content.
  useEffect(() => {
    let cancelled = false

    fetchAttachmentContent(url)
      .then((text) => {
        if (cancelled) return
        if (text.length > MAX_PREVIEW_BYTES) {
          setRawText(text.slice(0, MAX_PREVIEW_BYTES))
          setTruncated(true)
        } else {
          setRawText(text)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load')
      })

    return () => {
      cancelled = true
    }
  }, [url])

  // Highlight when text or theme changes.
  useEffect(() => {
    if (rawText == null) return
    let cancelled = false

    if (isMarkdown(mimeType, fileName)) {
      import('marked')
        .then(({ marked }) => marked(rawText))
        .then((rawHtml) => {
          if (cancelled) return
          const sanitized = DOMPurify.sanitize(rawHtml, { USE_PROFILES: { html: true } })
          setHtml(sanitized)
          setIsMarkdownContent(true)
        })
        .catch(() => {
          if (!cancelled) setHtml(`<pre>${DOMPurify.sanitize(rawText)}</pre>`)
        })
      return () => {
        cancelled = true
      }
    }

    const lang = detectLanguage(mimeType, fileName)
    const theme = resolvedTheme === 'dark' ? 'github-dark' : 'github-light'

    getHighlighter()
      .then((highlighter) => {
        if (cancelled) return
        const highlighted = highlighter.codeToHtml(rawText, { lang, theme })
        // Sanitize shiki output with DOMPurify to prevent XSS.
        const sanitized = DOMPurify.sanitize(highlighted, {
          USE_PROFILES: { html: true },
          ADD_TAGS: ['span', 'pre', 'code'],
          ADD_ATTR: ['class', 'style', 'tabindex'],
        })
        setHtml(sanitized)
      })
      .catch(() => {
        // Fallback: render raw text if highlighting fails.
        if (!cancelled) {
          setHtml(
            `<pre class="shiki" style="overflow-x:auto"><code>${DOMPurify.sanitize(rawText)}</code></pre>`,
          )
        }
      })

    return () => {
      cancelled = true
    }
  }, [rawText, mimeType, fileName, resolvedTheme])

  function handleCopy() {
    if (rawText == null) return
    navigator.clipboard.writeText(rawText).then(() => {
      setCopied(true)
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current)
      copyTimerRef.current = setTimeout(() => setCopied(false), 2000)
    })
  }

  if (error) {
    return (
      <div className="border-destructive/50 bg-destructive/10 text-destructive flex items-center gap-2 rounded-md border px-4 py-3 text-sm">
        <AlertTriangle className="h-4 w-4 shrink-0" />
        {error}
      </div>
    )
  }

  if (html == null) {
    return (
      <div className="space-y-2" data-testid="text-preview-loading">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-4 w-5/6" />
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-3/5" />
      </div>
    )
  }

  return (
    <div className="relative">
      <Button
        variant="ghost"
        size="sm"
        className="absolute top-2 right-2 z-10 h-7 gap-1 text-xs opacity-70 hover:opacity-100"
        onClick={handleCopy}
      >
        {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        {copied ? 'Copied' : 'Copy'}
      </Button>

      <div
        className={
          isMarkdownContent
            ? 'max-h-[70vh] overflow-auto rounded-md px-4 py-2 text-sm [&_h1]:mb-3 [&_h1]:text-lg [&_h1]:font-bold [&_h2]:mb-2 [&_h2]:text-base [&_h2]:font-semibold [&_h3]:mb-2 [&_h3]:text-sm [&_h3]:font-semibold [&_p]:mb-2 [&_ul]:mb-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:mb-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:mb-1 [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_pre]:mb-2 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-3 [&_blockquote]:border-l-2 [&_blockquote]:border-muted-foreground/30 [&_blockquote]:pl-3 [&_blockquote]:italic [&_a]:text-primary [&_a]:underline'
            : '[&_code_.line]:before:text-muted-foreground/50 [&_code]:counter-reset-[line] [&_code_.line]:counter-increment-[line] max-h-[70vh] overflow-auto rounded-md text-sm [&_code_.line]:before:mr-4 [&_code_.line]:before:inline-block [&_code_.line]:before:w-8 [&_code_.line]:before:text-right [&_code_.line]:before:content-[counter(line)] [&_pre]:!overflow-x-auto [&_pre]:!rounded-md [&_pre]:!p-4 [&_pre]:!whitespace-pre'
        }
        data-testid="text-preview-content"
        dangerouslySetInnerHTML={{ __html: html }}
      />

      {truncated && (
        <div className="mt-2 flex items-center gap-2 rounded-md bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
          File truncated — showing first 500 KB
        </div>
      )}
    </div>
  )
}
