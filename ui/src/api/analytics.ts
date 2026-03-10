import { apiClient } from './client'
import type {
  ApiResponse,
  AnalyticsResponse,
  ErrorCluster,
  SuitePassRate,
  LabelCount,
} from '@/types/api'

export async function fetchTopErrors(
  projectId: string,
  builds = 20,
  limit = 10,
): Promise<AnalyticsResponse<ErrorCluster>> {
  const res = await apiClient.get<ApiResponse<AnalyticsResponse<ErrorCluster>>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/errors`,
    { params: { builds, limit } },
  )
  return res.data.data
}

export async function fetchSuitePassRates(
  projectId: string,
  builds = 20,
): Promise<AnalyticsResponse<SuitePassRate>> {
  const res = await apiClient.get<ApiResponse<AnalyticsResponse<SuitePassRate>>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/suites`,
    { params: { builds } },
  )
  return res.data.data
}

export async function fetchLabelBreakdown(
  projectId: string,
  name: string,
  builds = 20,
): Promise<AnalyticsResponse<LabelCount>> {
  const res = await apiClient.get<ApiResponse<AnalyticsResponse<LabelCount>>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/labels`,
    { params: { name, builds } },
  )
  return res.data.data
}
