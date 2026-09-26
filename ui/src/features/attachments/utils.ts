import { isPlaywrightTrace } from '@/features/trace/utils'
import type { AttachmentEntry } from '@/types/api'

export function isLogMime(mimeType: string): boolean {
  return (
    mimeType.startsWith('text/') ||
    mimeType === 'application/json' ||
    mimeType === 'application/xml'
  )
}

/** Attachment-type filter values; '' shows every attachment. */
export type MimeFilter = '' | 'image' | 'text' | 'trace' | 'other'

export function filterAttachments(
  attachments: AttachmentEntry[],
  mimeFilter: MimeFilter,
): AttachmentEntry[] {
  if (mimeFilter === '') return attachments
  if (mimeFilter === 'image') return attachments.filter((a) => a.mime_type.startsWith('image/'))
  if (mimeFilter === 'text') return attachments.filter((a) => isLogMime(a.mime_type))
  if (mimeFilter === 'trace')
    return attachments.filter((a) => isPlaywrightTrace(a.name, a.mime_type))
  if (mimeFilter === 'other')
    return attachments.filter(
      (a) =>
        !a.mime_type.startsWith('image/') &&
        !isLogMime(a.mime_type) &&
        !isPlaywrightTrace(a.name, a.mime_type),
    )
  return attachments
}
