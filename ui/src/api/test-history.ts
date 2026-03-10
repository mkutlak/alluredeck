import { apiClient } from './client'
import type { ApiResponse, TestHistoryData } from '@/types/api'

export async function fetchTestHistory(
  projectId: string,
  historyId: string,
  branch?: string,
  limit?: number,
): Promise<TestHistoryData> {
  const res = await apiClient.get<ApiResponse<TestHistoryData>>(
    `/projects/${encodeURIComponent(projectId)}/tests/history`,
    {
      params: {
        history_id: historyId,
        ...(branch !== undefined ? { branch } : {}),
        ...(limit !== undefined ? { limit } : {}),
      },
    },
  )
  return res.data.data
}
