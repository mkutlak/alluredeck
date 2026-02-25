// ---------------------------------------------------------------------------
// Generic envelope
// ---------------------------------------------------------------------------
export interface ApiResponse<T> {
  data: T
  meta_data: {
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
  expires_in: number
  roles: string[]
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------
export interface ProjectEntry {
  project_id: string
}

export type ProjectsData = Record<string, ProjectEntry>

export interface CreateProjectRequest {
  id: string
}

export type CreateProjectData = Record<string, ProjectEntry>

// ---------------------------------------------------------------------------
// System
// ---------------------------------------------------------------------------
export interface VersionData {
  version: string
}

export interface ConfigData {
  version: string
  dev_mode: number
  check_results_every_seconds: string
  keep_history: number
  keep_history_latest: number
  tls: number
  security_enabled: number
  url_prefix: string
  api_response_less_verbose: number
  optimize_storage: number
  make_viewer_endpoints_public: number
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
