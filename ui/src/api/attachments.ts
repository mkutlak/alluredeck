import { apiClient } from './client'
import type { AttachmentsData, AttachmentGroup } from '@/types/api'

export interface FetchAttachmentsOptions {
  mimeType?: string
  limit?: number
  offset?: number
}

export async function fetchAttachments(
  projectId: string,
  reportId: string,
  opts?: FetchAttachmentsOptions,
): Promise<AttachmentsData> {
  const res = await apiClient.get<{ data: AttachmentsData }>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/attachments`,
    {
      params: {
        ...(opts?.mimeType ? { mime_type: opts.mimeType } : {}),
        ...(opts?.limit !== undefined ? { limit: opts.limit } : {}),
        ...(opts?.offset !== undefined ? { offset: opts.offset } : {}),
      },
    },
  )
  const data = res.data.data
  data.groups = data.groups.map((group: AttachmentGroup) => ({
    ...group,
    attachments: group.attachments.map((att) => ({
      ...att,
      url: attachmentFileUrl(projectId, reportId, att.source),
    })),
  }))
  return data
}

export function attachmentFileUrl(projectId: string, reportId: string, source: string): string {
  const base = apiClient.defaults.baseURL ?? ''
  return `${base}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/attachments/${encodeURIComponent(source)}`
}

/**
 * Fetch the raw text content of an attachment (for preview).
 */
export async function fetchAttachmentContent(url: string): Promise<string> {
  const res = await fetch(url, { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`Failed to fetch attachment: ${res.status}`)
  }
  return res.text()
}

/**
 * Download an attachment via fetch (works cross-origin with credentials).
 * The HTML `download` attribute is ignored for cross-origin URLs, so we
 * fetch the blob and trigger a download via a temporary object URL.
 */
export async function downloadAttachment(url: string, fileName: string): Promise<void> {
  const res = await fetch(`${url}?dl=1`, { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`Download failed: ${res.status}`)
  }
  const blob = await res.blob()
  const objUrl = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = objUrl
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(objUrl)
}
