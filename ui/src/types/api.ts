// ---------------------------------------------------------------------------
// Generic envelope
// ---------------------------------------------------------------------------
export interface ApiResponse<T> {
  data: T
  metadata: {
    message: string
  }
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------
export interface LoginRequest {
  username: string
  password: string
}

export interface LoginData {
  access_token: string
  refresh_token: string
  csrf_token?: string
  expires_in: number
  roles: string[]
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------
export interface ProjectEntry {
  project_id: string
  created_at?: string
}

export type ProjectsData = ProjectEntry[]

export interface PaginationMeta {
  page: number
  per_page: number
  total: number
  total_pages: number
}

export interface PaginatedResponse<T> {
  data: T
  metadata: {
    message: string
  }
  pagination: PaginationMeta
}

export interface CreateProjectRequest {
  id: string
}

export type CreateProjectData = ProjectEntry

// ---------------------------------------------------------------------------
// System
// ---------------------------------------------------------------------------
export interface VersionData {
  version: string
}

export interface ConfigData {
  version: string
  dev_mode: boolean
  check_results_every_seconds: string
  keep_history: boolean
  keep_history_latest: number
  tls: boolean
  security_enabled: boolean
  url_prefix: string
  api_response_less_verbose: boolean
  optimize_storage: boolean
  make_viewer_endpoints_public: boolean
}

// ---------------------------------------------------------------------------
// Reports
// ---------------------------------------------------------------------------
export interface GenerateReportParams {
  project_id: string
  execution_name?: string
  execution_from?: string
  execution_type?: string
  store_results?: boolean
}

export interface GenerateReportData {
  project_id: string
  output: string
}

export interface ResultFile {
  file_name: string
  content_base64: string
}

export interface SendResultsRequest {
  results: ResultFile[]
}

// ---------------------------------------------------------------------------
// Allure report summary – read from static report JSON files
// ---------------------------------------------------------------------------
export interface AllureStatistic {
  passed: number
  failed: number
  broken: number
  skipped: number
  unknown: number
  total: number
}

export interface AllureTime {
  start: number
  stop: number
  duration: number
  minDuration?: number
  maxDuration?: number
  sumDuration?: number
}

export interface AllureSummary {
  statistic: AllureStatistic
  time?: AllureTime
}

// ---------------------------------------------------------------------------
// Report history API response
// ---------------------------------------------------------------------------
export interface ReportHistoryEntry {
  report_id: string
  is_latest: boolean
  generated_at: string | null
  duration_ms: number | null
  statistic: AllureStatistic | null
}

export interface ReportHistoryData {
  project_id: string
  reports: ReportHistoryEntry[]
}

// ---------------------------------------------------------------------------
// Local report metadata (derived / assembled by the UI)
// ---------------------------------------------------------------------------
export interface ReportItem {
  reportId: string
  isLatest: boolean
  generatedAt?: string
  durationMs?: number
  summary?: AllureSummary
}
