import { apiClient } from './client'
import type {
  ApiResponse,
  CreateProjectData,
  CreateProjectRequest,
  PaginatedResponse,
  ProjectEntry,
  ProjectsData,
} from '@/types/api'

export async function getProjects(
  page?: number,
  perPage?: number,
): Promise<PaginatedResponse<ProjectsData>> {
  const res = await apiClient.get<PaginatedResponse<ProjectsData>>('/projects', {
    params: {
      ...(page ? { page } : {}),
      ...(perPage ? { per_page: perPage } : {}),
    },
  })
  return res.data
}

export async function createProject(
  payload: CreateProjectRequest,
): Promise<ApiResponse<CreateProjectData>> {
  const res = await apiClient.post<ApiResponse<CreateProjectData>>('/projects', payload)
  return res.data
}

export async function updateProjectTags(
  projectId: string,
  tags: string[],
): Promise<ApiResponse<ProjectEntry>> {
  const res = await apiClient.put<ApiResponse<ProjectEntry>>(
    `/projects/${projectId}/tags`,
    { tags },
  )
  return res.data
}

export async function getTags(): Promise<ApiResponse<string[]>> {
  const res = await apiClient.get<ApiResponse<string[]>>('/tags')
  return res.data
}
