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
  csrf_token?: string
  expires_in: number
  roles: string[]
  provider?: 'local' | 'oidc'
}

export interface SessionData {
  username: string
  roles: string[]
  expires_in: number
  provider: 'local' | 'oidc'
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------
export interface ProjectEntry {
  project_id: string
  created_at?: string
  parent_id?: string | null
  report_type?: 'allure' | 'playwright'
  children?: string[]
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
  parent_id?: string
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
  oidc_enabled: boolean
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

// ---------------------------------------------------------------------------
// Async job types
// ---------------------------------------------------------------------------
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed'

export interface JobData {
  job_id: string
  project_id: string
  status: JobStatus
  created_at: string
  started_at: string | null
  completed_at: string | null
  output: string
  error: string
}

// GenerateReportAccepted is the 202 response body
export interface GenerateReportAccepted {
  job_id: string
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
  has_playwright_report?: boolean
  flaky_count?: number
  retried_count?: number
  new_failed_count?: number
  new_passed_count?: number
  ci_provider?: string
  ci_build_url?: string
  ci_branch?: string
  ci_commit_sha?: string
}

export interface ReportHistoryData {
  project_id: string
  reports: ReportHistoryEntry[]
}

// ---------------------------------------------------------------------------
// Stability (A1)
// ---------------------------------------------------------------------------
export interface StabilityTestEntry {
  name: string
  full_name: string
  status: string
  retries_count: number
  retries_status_change: boolean
}

export interface StabilitySummary {
  flaky_count: number
  retried_count: number
  new_failed_count: number
  new_passed_count: number
  total: number
}

export interface StabilityData {
  flaky_tests: StabilityTestEntry[]
  new_failed: StabilityTestEntry[]
  new_passed: StabilityTestEntry[]
  summary: StabilitySummary
}

// ---------------------------------------------------------------------------
// Low Performing Tests (A2)
// ---------------------------------------------------------------------------
export interface LowPerformingTest {
  test_name: string
  full_name: string
  history_id: string
  metric: number
  build_count: number
  trend: number[]
}

export interface LowPerformingData {
  tests: LowPerformingTest[]
  sort: 'duration' | 'failure_rate'
  builds: number
  total: number
}

// ---------------------------------------------------------------------------
// Environment info (G1)
// ---------------------------------------------------------------------------
export interface EnvironmentEntry {
  name: string
  values: string[]
}

// ---------------------------------------------------------------------------
// Categories / defects (G2)
// ---------------------------------------------------------------------------
export interface CategoryMatchedStatistic {
  failed: number
  broken: number
  known: number
  unknown: number
  total: number
}

export interface CategoryEntry {
  name: string
  matchedStatistic: CategoryMatchedStatistic | null
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

// ---------------------------------------------------------------------------
// Known Failures (G5 analytics)
// ---------------------------------------------------------------------------
export interface KnownFailure {
  test_name: string
  status: string
}

export interface KnownFailuresData {
  known_failures: KnownFailure[]
  new_failures: KnownFailure[]
  adjusted_stats: {
    known_count: number
    new_count: number
    total_count: number
  }
}

// ---------------------------------------------------------------------------
// Known Issues (G5)
// ---------------------------------------------------------------------------
export interface KnownIssue {
  id: number
  project_id: string
  test_name: string
  pattern: string
  ticket_url: string
  description: string
  is_active: boolean
  created_at: string
  updated_at: string
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------
export interface SearchProjectMatch {
  project_id: string
  created_at: string
}

export interface SearchTestMatch {
  project_id: string
  test_name: string
  full_name: string
  status: string
}

export interface SearchData {
  projects: SearchProjectMatch[]
  tests: SearchTestMatch[]
}

// ---------------------------------------------------------------------------
// Timeline (G3)
// ---------------------------------------------------------------------------
export interface TimelineTestCase {
  name: string
  full_name: string
  status: string
  start: number
  stop: number
  duration: number
  thread: string
  host: string
}

export interface TimelineSummary {
  total: number
  min_start: number
  max_stop: number
  total_duration: number
  truncated: boolean
}

export interface TimelineData {
  test_cases: TimelineTestCase[]
  summary: TimelineSummary
}

export interface TimelineBuildEntry {
  build_order: number
  created_at: string
  test_cases: TimelineTestCase[]
  summary: TimelineSummary
}

export interface MultiTimelineData {
  builds: TimelineBuildEntry[]
  total_builds_in_range: number
  builds_returned: number
  global_min_start: number
  global_max_stop: number
}

// ---------------------------------------------------------------------------
// Build Comparison (Diff View)
// ---------------------------------------------------------------------------
export type DiffCategory = 'regressed' | 'fixed' | 'added' | 'removed'

export interface CompareDiffEntry {
  test_name: string
  full_name: string
  history_id: string
  status_a: string
  status_b: string
  duration_a: number
  duration_b: number
  duration_delta: number
  category: DiffCategory
}

export interface CompareSummary {
  regressed: number
  fixed: number
  added: number
  removed: number
  total: number
}

export interface CompareData {
  build_a: number
  build_b: number
  summary: CompareSummary
  tests: CompareDiffEntry[]
}

// ---------------------------------------------------------------------------
// Admin System Monitor
// ---------------------------------------------------------------------------
export type AdminJobStatus = 'pending' | 'running' | 'retrying' | 'completed' | 'failed' | 'cancelled'

export interface AdminJobEntry {
  job_id: string
  project_id: string
  status: AdminJobStatus
  created_at: string
  started_at: string | null
  completed_at: string | null
  output: string
  error: string
}

export interface AdminResultsEntry {
  project_id: string
  file_count: number
  total_size: number
  last_modified: string
}

// ---------------------------------------------------------------------------
// Dashboard (cross-project overview)
// ---------------------------------------------------------------------------
export interface DashboardLatestBuild {
  build_order: number
  created_at: string
  statistics: AllureStatistic
  pass_rate: number
  duration_ms: number
  flaky_count: number
  new_failed_count: number
  new_passed_count: number
  ci_branch?: string
}

export interface DashboardSparklinePoint {
  build_order: number
  pass_rate: number
}

export interface DashboardProjectEntry {
  project_id: string
  report_type?: 'allure' | 'playwright'
  created_at: string
  latest_build: DashboardLatestBuild | null
  sparkline: DashboardSparklinePoint[]
  is_group?: boolean
  children?: DashboardProjectEntry[]
  aggregate?: {
    passed: number
    failed: number
    broken: number
    skipped: number
    total: number
    pass_rate: number
  }
}

export interface DashboardSummary {
  total_projects: number
  healthy: number
  degraded: number
  failing: number
}

export interface DashboardData {
  projects: DashboardProjectEntry[]
  summary: DashboardSummary
}

// ---------------------------------------------------------------------------
// Branches
// ---------------------------------------------------------------------------
export interface Branch {
  id: number
  project_id: string
  name: string
  is_default: boolean
  created_at: string
}

// ---------------------------------------------------------------------------
// Test history
// ---------------------------------------------------------------------------
export interface TestHistoryEntry {
  build_order: number
  build_id: number
  status: string
  duration_ms: number
  created_at: string
  ci_commit_sha?: string
}

export interface TestHistoryData {
  history: TestHistoryEntry[]
  history_id: string
  branch_name?: string
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------
export type Role = 'admin' | 'editor' | 'viewer'

export interface APIKey {
  id: number
  name: string
  prefix: string
  role: Role
  expires_at: string | null
  last_used: string | null
  created_at: string
}

export interface APIKeyCreated extends APIKey {
  key: string // full key, shown once at creation
}

export interface CreateAPIKeyRequest {
  name: string
  expires_at?: string
}

// ---------------------------------------------------------------------------
// Analytics (Phase 8 — PostgreSQL analytics dashboards)
// ---------------------------------------------------------------------------
export interface ErrorCluster {
  message: string
  count: number
}

export interface SuitePassRate {
  suite: string
  total: number
  passed: number
  pass_rate: number
}

export interface LabelCount {
  value: string
  count: number
}

export interface AnalyticsResponse<T> {
  data: T[]
  metadata: { message: string }
}

// ---------------------------------------------------------------------------
// Analytics Trends (server-computed)
// ---------------------------------------------------------------------------
export interface TrendsStatusPoint {
  name: string
  passed: number
  failed: number
  broken: number
  skipped: number
}

export interface TrendsPassRatePoint {
  name: string
  pass_rate: number
}

export interface TrendsDurationPoint {
  name: string
  duration_sec: number
}

export interface TrendsKpi {
  pass_rate: number
  pass_rate_trend: number[]
  total_tests: number
  total_tests_trend: number[]
  avg_duration: number
  duration_trend: number[]
  failed_count: number
  failed_trend: number[]
}

export interface TrendsData {
  status: TrendsStatusPoint[]
  pass_rate: TrendsPassRatePoint[]
  duration: TrendsDurationPoint[]
  kpi: TrendsKpi | null
}

// ---------------------------------------------------------------------------
// Attachments (D2)
// ---------------------------------------------------------------------------
export interface AttachmentEntry {
  id: number
  name: string
  source: string
  mime_type: string
  size_bytes: number
  url: string
}

export interface AttachmentGroup {
  test_name: string
  test_status: string
  attachments: AttachmentEntry[]
}

export interface AttachmentsData {
  groups: AttachmentGroup[]
  total: number
  limit: number
  offset: number
}

// ---------------------------------------------------------------------------
// Defect Grouping
// ---------------------------------------------------------------------------
export type DefectCategory = 'product_bug' | 'test_bug' | 'infrastructure' | 'to_investigate'
export type DefectResolution = 'open' | 'fixed' | 'muted' | 'wont_fix'

export interface DefectFingerprint {
  id: string
  project_id: string
  fingerprint_hash: string
  normalized_message: string
  sample_trace: string
  category: DefectCategory
  resolution: DefectResolution
  known_issue_id: number | null
  first_seen_build_id: number
  last_seen_build_id: number
  occurrence_count: number
  consecutive_clean_builds: number
  created_at: string
  updated_at: string
}

export interface DefectListRow extends DefectFingerprint {
  test_result_count_in_build: number | null
  first_seen_build_order: number
  last_seen_build_order: number
  is_regression: boolean
  is_new: boolean
  known_issue: KnownIssue | null
}

export interface DefectBuildSummary {
  total_groups: number
  affected_tests: number
  new_defects: number
  regressions: number
  by_category: Record<string, number>
  by_resolution: Record<string, number>
}

export interface DefectProjectSummary {
  open: number
  fixed: number
  muted: number
  wont_fix: number
  regressions_last_build: number
  by_category: Record<string, number>
}

export interface DefectListResponse {
  data: DefectListRow[]
  metadata: { message: string }
  pagination: PaginationMeta
}

// ---------------------------------------------------------------------------
// Webhook types
// ---------------------------------------------------------------------------
export type WebhookTargetType = 'slack' | 'discord' | 'teams' | 'generic'

export interface Webhook {
  id: string
  project_id: string
  name: string
  target_type: WebhookTargetType
  url: string
  has_secret: boolean
  template: string | null
  events: string[]
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  webhook_id: string
  build_id: number | null
  event: string
  status_code: number | null
  response_body: string | null
  error: string | null
  attempt: number
  duration_ms: number | null
  delivered_at: string
}

export interface CreateWebhookRequest {
  name: string
  target_type: string
  url: string
  secret?: string
  template?: string
  events?: string[]
  is_active?: boolean
}

export interface UpdateWebhookRequest {
  name?: string
  target_type?: string
  url?: string
  secret?: string
  template?: string
  events?: string[]
  is_active?: boolean
}

// ---------------------------------------------------------------------------
// Pipeline Runs
// ---------------------------------------------------------------------------

export interface PipelineSuite {
  project_id: string
  build_order: number
  pass_rate: number
  total: number
  failed: number
  duration_ms: number
  status: 'passed' | 'failed' | 'degraded'
}

export interface PipelineAggregate {
  suites_passed: number
  suites_total: number
  tests_passed: number
  tests_total: number
  pass_rate: number
  total_duration_ms: number
}

export interface PipelineRun {
  commit_sha: string
  branch: string
  ci_build_url?: string
  timestamp: string
  suites: PipelineSuite[]
  aggregate: PipelineAggregate
}
