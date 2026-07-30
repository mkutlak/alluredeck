import { apiClient } from './client'
import type { PaginatedResponse, PipelineRun, RunFailuresResponse } from '@/types/api'

/**
 * Fetches every failing test in one pipeline run. runKey is the run's
 * ci_pipeline_id, or its commit SHA when the pipeline ID is absent — the same
 * key the API groups runs by. projectId is the parent (group) project.
 */
export async function fetchRunFailures(
  projectId: number,
  runKey: string,
  limit?: number,
): Promise<RunFailuresResponse> {
  const res = await apiClient.get<RunFailuresResponse>(
    `/projects/${encodeURIComponent(projectId)}/pipeline-runs/${encodeURIComponent(runKey)}/failures`,
    { params: limit !== undefined ? { limit } : {} },
  )
  return res.data
}

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

/**
 * Fetches the cross-group runs feed. group_id is a repeatable query param,
 * which `apiClient`'s params serializer cannot express (it stringifies each
 * value once) — so the query string is built manually here.
 */
export async function fetchRunsFeed(
  page?: number,
  perPage?: number,
  branch?: string,
  groupIds?: number[],
): Promise<PaginatedResponse<PipelineRun[]>> {
  const searchParams = new URLSearchParams()
  if (page !== undefined) searchParams.append('page', String(page))
  if (perPage !== undefined) searchParams.append('per_page', String(perPage))
  if (branch !== undefined) searchParams.append('branch', branch)
  if (groupIds) {
    for (const id of groupIds) {
      searchParams.append('group_id', String(id))
    }
  }
  const qs = searchParams.toString()
  const url = qs ? `/pipeline-runs?${qs}` : '/pipeline-runs'
  const res = await apiClient.get<PaginatedResponse<PipelineRun[]>>(url)
  return res.data
}
