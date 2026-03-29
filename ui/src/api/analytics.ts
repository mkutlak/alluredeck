import { apiClient } from './client'
import type {
  AnalyticsResponse,
  ErrorCluster,
  SuitePassRate,
  LabelCount,
  TrendsData,
} from '@/types/api'

export async function fetchTopErrors(
  projectId: string,
  builds = 20,
  limit = 10,
  branch?: string,
): Promise<AnalyticsResponse<ErrorCluster>> {
  const res = await apiClient.get<AnalyticsResponse<ErrorCluster>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/errors`,
    { params: { builds, limit, ...(branch ? { branch } : {}) } },
  )
  return res.data
}

export async function fetchSuitePassRates(
  projectId: string,
  builds = 20,
  branch?: string,
): Promise<AnalyticsResponse<SuitePassRate>> {
  const res = await apiClient.get<AnalyticsResponse<SuitePassRate>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/suites`,
    { params: { builds, ...(branch ? { branch } : {}) } },
  )
  return res.data
}

export async function fetchLabelBreakdown(
  projectId: string,
  name: string,
  builds = 20,
  branch?: string,
): Promise<AnalyticsResponse<LabelCount>> {
  const res = await apiClient.get<AnalyticsResponse<LabelCount>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/labels`,
    { params: { name, builds, ...(branch ? { branch } : {}) } },
  )
  return res.data
}

export async function fetchTrends(
  projectId: string,
  builds = 100,
  branch?: string,
): Promise<TrendsData> {
  const res = await apiClient.get<{ data: TrendsData; metadata: { message: string } }>(
    `/projects/${encodeURIComponent(projectId)}/analytics/trends`,
    { params: { builds, ...(branch ? { branch } : {}) } },
  )
  return res.data.data
}
