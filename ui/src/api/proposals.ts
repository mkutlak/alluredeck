import { apiClient } from './client'
import type {
  DefectProposal,
  FlakyProposal,
  KnownIssueProposal,
  ProposalListResponse,
  ProposalStatus,
  ProposalType,
} from '@/types/proposals'

export async function listDefectProposals(params: {
  project_id: number
  status?: ProposalStatus
  limit?: number
  cursor?: string
}): Promise<ProposalListResponse<DefectProposal>> {
  const res = await apiClient.get<ProposalListResponse<DefectProposal>>('/proposals/defect', {
    params: {
      project_id: params.project_id,
      ...(params.status !== undefined ? { status: params.status } : {}),
      ...(params.limit !== undefined ? { limit: params.limit } : {}),
      ...(params.cursor !== undefined ? { cursor: params.cursor } : {}),
    },
  })
  return res.data
}

export async function listKnownIssueProposals(params: {
  project_id: number
  status?: ProposalStatus
  limit?: number
  cursor?: string
}): Promise<ProposalListResponse<KnownIssueProposal>> {
  const res = await apiClient.get<ProposalListResponse<KnownIssueProposal>>(
    '/proposals/known_issue',
    {
      params: {
        project_id: params.project_id,
        ...(params.status !== undefined ? { status: params.status } : {}),
        ...(params.limit !== undefined ? { limit: params.limit } : {}),
        ...(params.cursor !== undefined ? { cursor: params.cursor } : {}),
      },
    },
  )
  return res.data
}

export async function listFlakyProposals(params: {
  project_id: number
  status?: ProposalStatus
  limit?: number
  cursor?: string
}): Promise<ProposalListResponse<FlakyProposal>> {
  const res = await apiClient.get<ProposalListResponse<FlakyProposal>>('/proposals/flaky', {
    params: {
      project_id: params.project_id,
      ...(params.status !== undefined ? { status: params.status } : {}),
      ...(params.limit !== undefined ? { limit: params.limit } : {}),
      ...(params.cursor !== undefined ? { cursor: params.cursor } : {}),
    },
  })
  return res.data
}

export async function approveProposal(
  type: ProposalType,
  id: number,
  body?: { reason?: string },
): Promise<void> {
  await apiClient.post(`/proposals/${type}/${id}/approve`, body ?? {})
}

export async function rejectProposal(
  type: ProposalType,
  id: number,
  body: { reason: string },
): Promise<void> {
  await apiClient.post(`/proposals/${type}/${id}/reject`, body)
}
