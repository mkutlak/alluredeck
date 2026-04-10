import { apiClient } from '@/api/client'
import type { AdminJobEntry, AdminResultsEntry, ApiResponse } from '@/types/api'

export async function fetchAdminJobs(): Promise<AdminJobEntry[]> {
  const res = await apiClient.get<ApiResponse<AdminJobEntry[]>>('/admin/jobs')
  return res.data.data
}

export async function fetchAdminResults(): Promise<AdminResultsEntry[]> {
  const res = await apiClient.get<ApiResponse<AdminResultsEntry[]>>('/admin/results')
  return res.data.data
}

export async function cancelJob(jobId: string): Promise<void> {
  await apiClient.post(`/admin/jobs/${encodeURIComponent(jobId)}/cancel`)
}

export async function cleanAdminResults(projectId: string): Promise<void> {
  await apiClient.delete(`/admin/results/${encodeURIComponent(projectId)}`)
}

export async function deleteJob(jobId: string): Promise<void> {
  await apiClient.delete(`/admin/jobs/${encodeURIComponent(jobId)}`)
}

export async function cleanAdminResultsBulk(projectIds: number[]): Promise<void> {
  await apiClient.delete('/admin/results', { project_ids: projectIds })
}
