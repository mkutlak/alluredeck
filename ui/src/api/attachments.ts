import { apiClient } from './client'
import type { AttachmentsData } from '@/types/api'

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
  return res.data.data
}

export function attachmentFileUrl(
  projectId: string,
  reportId: string,
  source: string,
): string {
  const base = apiClient.defaults.baseURL ?? ''
  return `${base}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/attachments/${encodeURIComponent(source)}`
}
