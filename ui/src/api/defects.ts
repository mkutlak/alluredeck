import { apiClient } from './client'
import type {
  ApiResponse,
  DefectBuildSummary,
  DefectFingerprint,
  DefectListResponse,
  DefectProjectSummary,
  DefectCategory,
  DefectResolution,
} from '@/types/api'

export interface DefectFilters {
  category?: DefectCategory
  resolution?: DefectResolution
  sort?: string
  search?: string
  page?: number
  per_page?: number
}

export async function fetchProjectDefects(
  projectId: string,
  filters?: DefectFilters,
): Promise<DefectListResponse> {
  const params: Record<string, unknown> = {}
  if (filters?.category) params.category = filters.category
  if (filters?.resolution) params.resolution = filters.resolution
  if (filters?.sort) params.sort = filters.sort
  if (filters?.search) params.search = filters.search
  if (filters?.page) params.page = filters.page
  if (filters?.per_page) params.per_page = filters.per_page

  const res = await apiClient.get<DefectListResponse>(
    `/projects/${encodeURIComponent(projectId)}/defects`,
    { params },
  )
  return res.data
}

export async function fetchBuildDefects(
  projectId: string,
  buildId: number,
  filters?: DefectFilters,
): Promise<DefectListResponse> {
  const params: Record<string, unknown> = {}
  if (filters?.category) params.category = filters.category
  if (filters?.resolution) params.resolution = filters.resolution
  if (filters?.sort) params.sort = filters.sort
  if (filters?.search) params.search = filters.search
  if (filters?.page) params.page = filters.page
  if (filters?.per_page) params.per_page = filters.per_page

  const res = await apiClient.get<DefectListResponse>(
    `/projects/${encodeURIComponent(projectId)}/builds/${buildId}/defects`,
    { params },
  )
  return res.data
}

export async function fetchDefect(projectId: string, defectId: string): Promise<DefectFingerprint> {
  const res = await apiClient.get<ApiResponse<DefectFingerprint>>(
    `/projects/${encodeURIComponent(projectId)}/defects/${encodeURIComponent(defectId)}`,
  )
  return res.data.data
}

export async function fetchDefectTests(
  projectId: string,
  defectId: string,
  buildId?: number,
  page?: number,
  perPage?: number,
): Promise<unknown> {
  const params: Record<string, unknown> = {}
  if (buildId != null) params.build_id = buildId
  if (page != null) params.page = page
  if (perPage != null) params.per_page = perPage

  const res = await apiClient.get<ApiResponse<unknown>>(
    `/projects/${encodeURIComponent(projectId)}/defects/${encodeURIComponent(defectId)}/tests`,
    { params },
  )
  return res.data.data
}

export async function updateDefect(
  projectId: string,
  defectId: string,
  data: { category?: DefectCategory; resolution?: DefectResolution },
): Promise<DefectFingerprint> {
  const res = await apiClient.patch<ApiResponse<DefectFingerprint>>(
    `/projects/${encodeURIComponent(projectId)}/defects/${encodeURIComponent(defectId)}`,
    data,
  )
  return res.data.data
}

export async function bulkUpdateDefects(
  projectId: string,
  data: { ids: string[]; category?: DefectCategory; resolution?: DefectResolution },
): Promise<void> {
  await apiClient.post(`/projects/${encodeURIComponent(projectId)}/defects/bulk`, data)
}

export async function fetchProjectDefectSummary(projectId: string): Promise<DefectProjectSummary> {
  const res = await apiClient.get<ApiResponse<DefectProjectSummary>>(
    `/projects/${encodeURIComponent(projectId)}/defects/summary`,
  )
  return res.data.data
}

export async function fetchBuildDefectSummary(
  projectId: string,
  buildId: number,
): Promise<DefectBuildSummary> {
  const res = await apiClient.get<ApiResponse<DefectBuildSummary>>(
    `/projects/${encodeURIComponent(projectId)}/builds/${buildId}/defects/summary`,
  )
  return res.data.data
}
