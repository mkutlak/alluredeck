export type ProposalStatus = 'pending' | 'approved' | 'rejected'
export type ProposalType = 'defect' | 'known_issue' | 'flaky'

interface ProposalBase {
  id: number
  project_id: number
  proposer_user_id: number
  proposer_api_key_id?: number
  status: ProposalStatus
  reviewed_by_user_id?: number
  reviewed_at?: string // ISO timestamp
  created_at: string
  rationale?: string
}

export interface DefectProposal extends ProposalBase {
  fingerprint_hash: string
  proposed_category: string
  proposed_resolution?: string
}

export interface KnownIssueProposal extends ProposalBase {
  error_message_sample: string
  proposed_category: string
  regex_pattern: string
  applies_to_status: string[]
  dry_run_match_count: number
}

export interface FlakyProposal extends ProposalBase {
  test_full_name: string
  history_id: string
}

export interface ProposalListResponse<T> {
  items: T[]
  next_cursor: string
}
