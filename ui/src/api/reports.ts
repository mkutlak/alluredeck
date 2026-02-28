import { apiClient } from './client'
import type { ApiResponse, AllureSummary, CategoryEntry, EnvironmentEntry, GenerateReportData, GenerateReportParams, KnownFailuresData, LowPerformingData, ReportHistoryData, StabilityData, TimelineData } from '@/types/api'
import { env } from '@/lib/env'

export async function generateReport(
  params: GenerateReportParams,
): Promise<ApiResponse<GenerateReportData>> {
  const { project_id, execution_name, execution_from, execution_type, store_results } = params
  const res = await apiClient.post<ApiResponse<GenerateReportData>>(
    `/projects/${encodeURIComponent(project_id)}/reports`,
    null,
    {
      params: {
        ...(execution_name ? { execution_name } : {}),
        ...(execution_from ? { execution_from } : {}),
        ...(execution_type ? { execution_type } : {}),
        ...(store_results !== undefined ? { store_results: store_results ? '1' : '0' } : {}),
      },
    },
  )
  return res.data
}

export async function cleanHistory(projectId: string): Promise<ApiResponse<{ output: string }>> {
  const res = await apiClient.delete<ApiResponse<{ output: string }>>(
    `/projects/${encodeURIComponent(projectId)}/reports/history`,
  )
  return res.data
}

export async function cleanResults(projectId: string): Promise<ApiResponse<{ output: string }>> {
  const res = await apiClient.delete<ApiResponse<{ output: string }>>(
    `/projects/${encodeURIComponent(projectId)}/results`,
  )
  return res.data
}

export async function sendResultsMultipart(projectId: string, files: File[]): Promise<void> {
  const formData = new FormData()
  files.forEach((file) => formData.append('files[]', file))
  await apiClient.post(`/projects/${encodeURIComponent(projectId)}/results`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

/** Build the URL for the emailable report page (GET, rendered as HTML). */
export function getEmailableReportUrl(projectId: string): string {
  return `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/emailable`
}

export async function deleteReport(
  projectId: string,
  reportId: string,
): Promise<ApiResponse<{ report_id: string; project_id: string }>> {
  const res = await apiClient.delete<ApiResponse<{ report_id: string; project_id: string }>>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}`,
  )
  return res.data
}

export async function fetchReportHistory(projectId: string): Promise<ReportHistoryData> {
  const res = await apiClient.get<ApiResponse<ReportHistoryData>>(
    `/projects/${encodeURIComponent(projectId)}/reports`,
  )
  return res.data.data
}

/** Attempt to load Allure summary JSON from the static report files. */
export async function fetchReportSummary(
  projectId: string,
  reportId: string,
): Promise<AllureSummary | null> {
  // Allure stores widget data at widgets/summary.json inside the report directory
  const url = `${env.apiUrl}/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/widgets/summary.json`
  try {
    const res = await fetch(url)
    if (!res.ok) return null
    return (await res.json()) as AllureSummary
  } catch {
    return null
  }
}

export async function fetchReportCategories(
  projectId: string,
  reportId = 'latest',
): Promise<CategoryEntry[]> {
  const res = await apiClient.get<ApiResponse<CategoryEntry[]>>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/categories`,
  )
  return res.data.data
}

export async function fetchReportEnvironment(
  projectId: string,
  reportId = 'latest',
): Promise<EnvironmentEntry[]> {
  const res = await apiClient.get<ApiResponse<EnvironmentEntry[]>>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/environment`,
  )
  return res.data.data
}

export async function fetchReportKnownFailures(
  projectId: string,
  reportId = 'latest',
): Promise<KnownFailuresData> {
  const res = await apiClient.get<ApiResponse<KnownFailuresData>>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/known-failures`,
  )
  return res.data.data
}

export async function fetchReportTimeline(
  projectId: string,
  reportId = 'latest',
): Promise<TimelineData> {
  const res = await apiClient.get<ApiResponse<TimelineData>>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/timeline`,
  )
  return res.data.data
}

export async function fetchReportStability(
  projectId: string,
  reportId = 'latest',
): Promise<StabilityData> {
  const res = await apiClient.get<ApiResponse<StabilityData>>(
    `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/stability`,
  )
  return res.data.data
}

export async function fetchLowPerformingTests(
  projectId: string,
  sort: 'duration' | 'failure_rate' = 'duration',
  builds = 20,
  limit = 20,
): Promise<LowPerformingData> {
  const res = await apiClient.get<ApiResponse<LowPerformingData>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/low-performing`,
    { params: { sort, builds, limit } },
  )
  return res.data.data
}
