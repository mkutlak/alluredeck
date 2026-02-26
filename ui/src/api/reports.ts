import { apiClient } from './client'
import type { ApiResponse, AllureSummary, GenerateReportData, GenerateReportParams, ReportHistoryData } from '@/types/api'
import { env } from '@/lib/env'

export async function generateReport(
  params: GenerateReportParams,
): Promise<ApiResponse<GenerateReportData>> {
  const { project_id, execution_name, execution_from, execution_type, store_results } = params
  const res = await apiClient.post<ApiResponse<GenerateReportData>>('/generate-report', null, {
    params: {
      project_id,
      ...(execution_name ? { execution_name } : {}),
      ...(execution_from ? { execution_from } : {}),
      ...(execution_type ? { execution_type } : {}),
      ...(store_results !== undefined ? { store_results: store_results ? '1' : '0' } : {}),
    },
  })
  return res.data
}

export async function cleanHistory(projectId: string): Promise<ApiResponse<{ output: string }>> {
  const res = await apiClient.delete<ApiResponse<{ output: string }>>(
    `/projects/${encodeURIComponent(projectId)}/history`,
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
  await apiClient.post('/send-results', formData, {
    params: { project_id: projectId },
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

/** Build the URL for the emailable report page (GET, rendered as HTML). */
export function getEmailableReportUrl(projectId: string): string {
  return `${env.apiUrl}/emailable-report/render?project_id=${encodeURIComponent(projectId)}`
}

export async function deleteReport(
  projectId: string,
  reportId: string,
): Promise<ApiResponse<{ report_id: string; project_id: string }>> {
  const res = await apiClient.delete<ApiResponse<{ report_id: string; project_id: string }>>(
    '/report',
    { params: { project_id: projectId, report_id: reportId } },
  )
  return res.data
}

export async function fetchReportHistory(projectId: string): Promise<ReportHistoryData> {
  const res = await apiClient.get<ApiResponse<ReportHistoryData>>('/report-history', {
    params: { project_id: projectId },
  })
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
