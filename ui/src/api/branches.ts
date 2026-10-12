import { apiClient } from './client'
import type { ApiResponse, Branch } from '@/types/api'

export async function fetchBranches(projectId: string): Promise<Branch[]> {
  const res = await apiClient.get<ApiResponse<{ branches: Branch[] }>>(
    `/projects/${encodeURIComponent(projectId)}/branches`,
  )
  return res.data.data.branches
}
