import { apiClient } from './client'
import type { ApiResponse, KnownIssue } from '@/types/api'

export async function listKnownIssues(
  projectId: string,
  activeOnly = false,
): Promise<KnownIssue[]> {
  const res = await apiClient.get<ApiResponse<KnownIssue[]>>(
    `/projects/${encodeURIComponent(projectId)}/known-issues`,
    { params: activeOnly ? { active_only: 'true' } : {} },
  )
  return res.data.data
}

export async function createKnownIssue(
  projectId: string,
  data: { test_name: string; pattern?: string; ticket_url?: string; description?: string },
): Promise<KnownIssue> {
  const res = await apiClient.post<ApiResponse<KnownIssue>>(
    `/projects/${encodeURIComponent(projectId)}/known-issues`,
    data,
  )
  return res.data.data
}

export async function updateKnownIssue(
  projectId: string,
  issueId: number,
  data: { ticket_url?: string; description?: string; is_active?: boolean },
): Promise<KnownIssue> {
  const res = await apiClient.put<ApiResponse<KnownIssue>>(
    `/projects/${encodeURIComponent(projectId)}/known-issues/${issueId}`,
    data,
  )
  return res.data.data
}

export async function deleteKnownIssue(projectId: string, issueId: number): Promise<void> {
  await apiClient.delete(
    `/projects/${encodeURIComponent(projectId)}/known-issues/${issueId}`,
  )
}
