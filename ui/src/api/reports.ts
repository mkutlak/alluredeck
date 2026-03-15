import { apiClient } from './client'
import type {
  ApiResponse,
  PaginatedResponse,
  AllureSummary,
  CategoryEntry,
  CompareData,
  EnvironmentEntry,
  GenerateReportAccepted,
  GenerateReportParams,
  JobData,
  KnownFailuresData,
  LowPerformingData,
  ReportHistoryData,
  StabilityData,
  TimelineData,
} from '@/types/api'

export async function generateReport(
  params: GenerateReportParams,
): Promise<ApiResponse<GenerateReportAccepted>> {
  const { project_id, execution_name, execution_from, execution_type, store_results } = params
  const res = await apiClient.post<ApiResponse<GenerateReportAccepted>>(
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

export async function getJobStatus(
  projectId: string,
  jobId: string,
): Promise<ApiResponse<JobData>> {
  const res = await apiClient.get<ApiResponse<JobData>>(
    `/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}`,
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

const TARGZ_EXTENSIONS = ['.tar.gz', '.tgz']

function isTarGzFile(file: File): boolean {
  const name = file.name.toLowerCase()
  return TARGZ_EXTENSIONS.some((ext) => name.endsWith(ext))
}

export async function sendResultsMultipart(projectId: string, files: File[]): Promise<void> {
  const url = `/projects/${encodeURIComponent(projectId)}/results`

  // Single tar.gz/tgz file → send as raw gzip body so the backend extracts it.
  if (files.length === 1 && isTarGzFile(files[0])) {
    await apiClient.post(url, files[0], {
      headers: { 'Content-Type': 'application/gzip' },
    })
    return
  }

  const formData = new FormData()
  files.forEach((file) => formData.append('files[]', file))
  await apiClient.post(url, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
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

export async function fetchReportHistory(
  projectId: string,
  page = 1,
  perPage = 20,
  branch?: string,
): Promise<PaginatedResponse<ReportHistoryData>> {
  const res = await apiClient.get<PaginatedResponse<ReportHistoryData>>(
    `/projects/${encodeURIComponent(projectId)}/reports`,
    {
      params: {
        page,
        per_page: perPage,
        ...(branch !== undefined ? { branch } : {}),
      },
    },
  )
  return res.data
}

/** Attempt to load Allure summary JSON from the static report files. */
export async function fetchReportSummary(
  projectId: string,
  reportId: string,
): Promise<AllureSummary | null> {
  try {
    const res = await apiClient.get<AllureSummary>(
      `/projects/${encodeURIComponent(projectId)}/reports/${encodeURIComponent(reportId)}/widgets/summary.json`,
    )
    return res.data
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
  branch?: string,
): Promise<LowPerformingData> {
  const res = await apiClient.get<ApiResponse<LowPerformingData>>(
    `/projects/${encodeURIComponent(projectId)}/analytics/low-performing`,
    { params: { sort, builds, limit, ...(branch ? { branch } : {}) } },
  )
  return res.data.data
}

export async function fetchBuildComparison(
  projectId: string,
  buildA: number,
  buildB: number,
): Promise<CompareData> {
  const res = await apiClient.get<ApiResponse<CompareData>>(
    `/projects/${encodeURIComponent(projectId)}/compare`,
    { params: { a: buildA, b: buildB } },
  )
  return res.data.data
}
