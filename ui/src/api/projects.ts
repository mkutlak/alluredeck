import { apiClient } from './client'
import type {
  ApiResponse,
  CreateProjectData,
  CreateProjectRequest,
  PaginatedResponse,
  ProjectEntry,
  ProjectsData,
} from '@/types/api'

export async function getProject(
  idOrSlug: string,
): Promise<ApiResponse<ProjectEntry>> {
  const res = await apiClient.get<ApiResponse<ProjectEntry>>(
    `/projects/${encodeURIComponent(idOrSlug)}`,
  )
  return res.data
}

export async function getProjectIndex(): Promise<ApiResponse<ProjectsData>> {
  const res = await apiClient.get<ApiResponse<ProjectsData>>('/projects/index')
  return res.data
}

export async function getProjects(
  page?: number,
  perPage?: number,
): Promise<PaginatedResponse<ProjectsData>> {
  const res = await apiClient.get<PaginatedResponse<ProjectsData>>('/projects', {
    params: {
      ...(page !== undefined ? { page } : {}),
      ...(perPage !== undefined ? { per_page: perPage } : {}),
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

export async function deleteProject(projectId: string | number): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(projectId)}`)
}

export async function renameProject(projectId: string | number, newId: string): Promise<void> {
  await apiClient.put(`/projects/${encodeURIComponent(projectId)}/rename`, { new_id: newId })
}

export async function setProjectParent(projectId: string | number, parentId: string | number): Promise<void> {
  await apiClient.put(`/projects/${encodeURIComponent(projectId)}/parent`, { parent_id: Number(parentId) })
}

export async function clearProjectParent(projectId: string | number): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(projectId)}/parent`)
}
