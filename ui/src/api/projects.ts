import { apiClient } from './client'
import type {
  ApiResponse,
  CreateProjectData,
  CreateProjectRequest,
  ProjectsData,
} from '@/types/api'

export async function getProjects(): Promise<ApiResponse<ProjectsData>> {
  const res = await apiClient.get<ApiResponse<ProjectsData>>('/projects')
  return res.data
}

export async function createProject(
  payload: CreateProjectRequest,
): Promise<ApiResponse<CreateProjectData>> {
  const res = await apiClient.post<ApiResponse<CreateProjectData>>('/projects', payload)
  return res.data
}
