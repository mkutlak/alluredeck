import { apiClient } from './client'
import type { ApiResponse, FailureSummaryData } from '@/types/api'

export async function fetchFailureSummary(
  projectId: number,
  buildId: number,
  historyId: string,
): Promise<FailureSummaryData> {
  const res = await apiClient.get<ApiResponse<FailureSummaryData>>(
    `/projects/${projectId}/builds/${buildId}/tests/${encodeURIComponent(historyId)}/failure-summary`,
  )
  return res.data.data
}
