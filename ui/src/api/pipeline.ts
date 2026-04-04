import { apiClient } from './client'
import type { PaginatedResponse, PipelineRun } from '@/types/api'

export async function fetchPipelineRuns(
  projectId: string,
  page?: number,
  perPage?: number,
  branch?: string,
): Promise<PaginatedResponse<PipelineRun[]>> {
  const res = await apiClient.get<PaginatedResponse<PipelineRun[]>>(
    `/projects/${encodeURIComponent(projectId)}/pipeline-runs`,
    {
      params: {
        ...(page !== undefined ? { page } : {}),
        ...(perPage !== undefined ? { per_page: perPage } : {}),
        ...(branch !== undefined ? { branch } : {}),
      },
    },
  )
  return res.data
}
