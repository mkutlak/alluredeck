import { apiClient } from './client'
import type { ApiResponse, Branch } from '@/types/api'

export async function fetchBranches(projectId: string): Promise<Branch[]> {
  const res = await apiClient.get<ApiResponse<{ branches: Branch[] }>>(
    `/projects/${encodeURIComponent(projectId)}/branches`,
  )
  return res.data.data.branches
}

export async function setDefaultBranch(projectId: string, branchId: number): Promise<void> {
  await apiClient.put(`/projects/${encodeURIComponent(projectId)}/branches/${branchId}/default`)
}

export async function deleteBranch(projectId: string, branchId: number): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(projectId)}/branches/${branchId}`)
}
