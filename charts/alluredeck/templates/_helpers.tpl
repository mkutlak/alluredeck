{{/*
Expand the name of the chart.
*/}}
{{- define "alluredeck.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "alluredeck.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
API component fullname.
*/}}
{{- define "alluredeck.api.fullname" -}}
{{- printf "%s-api" (include "alluredeck.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
UI component fullname.
*/}}
{{- define "alluredeck.ui.fullname" -}}
{{- printf "%s-ui" (include "alluredeck.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "alluredeck.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "alluredeck.labels" -}}
helm.sh/chart: {{ include "alluredeck.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end }}

{{/*
API labels.
*/}}
{{- define "alluredeck.api.labels" -}}
{{ include "alluredeck.labels" . }}
{{ include "alluredeck.api.selectorLabels" . }}
{{- end }}

{{/*
UI labels.
*/}}
{{- define "alluredeck.ui.labels" -}}
{{ include "alluredeck.labels" . }}
{{ include "alluredeck.ui.selectorLabels" . }}
{{- end }}

{{/*
API selector labels.
*/}}
{{- define "alluredeck.api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "alluredeck.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: api
{{- end }}

{{/*
UI selector labels.
*/}}
{{- define "alluredeck.ui.selectorLabels" -}}
app.kubernetes.io/name: {{ include "alluredeck.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: ui
{{- end }}

{{/*
Create the name of the API service account to use.
*/}}
{{- define "alluredeck.api.serviceAccountName" -}}
{{- if .Values.api.serviceAccount.create }}
{{- default (printf "%s-api" (include "alluredeck.fullname" .)) .Values.api.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.api.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the UI service account to use.
*/}}
{{- define "alluredeck.ui.serviceAccountName" -}}
{{- if .Values.ui.serviceAccount.create }}
{{- default (printf "%s-ui" (include "alluredeck.fullname" .)) .Values.ui.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.ui.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
API Secret name — use existingSecret if set, otherwise generate name.
*/}}
{{- define "alluredeck.api.secretName" -}}
{{- if .Values.api.security.existingSecret }}
{{- .Values.api.security.existingSecret }}
{{- else }}
{{- include "alluredeck.api.fullname" . }}
{{- end }}
{{- end }}

{{/*
Render the full API config.yaml content for the ConfigMap.
Non-secret settings only; secrets are injected as env vars from the Secret.
CORS: uses api.config.corsAllowedOrigins list if set, otherwise auto-derives
a single origin from ingress.host.
*/}}
{{- define "alluredeck.api.configYAML" -}}
port: {{ .Values.api.service.port | quote }}
dev_mode: {{ .Values.api.config.devMode }}
security_enabled: {{ .Values.api.security.enabled }}
log_level: {{ .Values.api.config.logLevel | quote }}
storage_type: {{ .Values.api.config.storageType | quote }}
check_results_every_secs: {{ .Values.api.config.checkResultsEverySeconds | quote }}
keep_history: {{ .Values.api.config.keepHistory }}
keep_history_latest: {{ .Values.api.config.keepHistoryLatest | int }}
api_response_less_verbose: {{ .Values.api.config.apiResponseLessVerbose }}
trust_forwarded_for: {{ .Values.api.config.trustXForwardedFor }}
swagger_enabled: {{ .Values.api.config.swaggerEnabled }}
make_viewer_endpoints_public: {{ .Values.api.config.makeViewerEndpointsPublic }}
projects_path: {{ .Values.api.config.staticContentProjects | quote }}
{{- if .Values.api.config.databaseURL }}
database_url: {{ .Values.api.config.databaseURL | quote }}
{{- end }}
max_upload_size_mb: {{ .Values.api.config.maxUploadSizeMb | int }}
{{- $corsOrigins := .Values.api.config.corsAllowedOrigins }}
{{- if and (empty $corsOrigins) .Values.ingress.enabled .Values.ingress.host }}
{{- $scheme := ternary "https" "http" (not (empty .Values.ingress.tls)) }}
{{- $corsOrigins = list (printf "%s://%s" $scheme .Values.ingress.host) }}
{{- end }}
{{- if $corsOrigins }}
cors_allowed_origins:
{{- range $corsOrigins }}
  - {{ . | quote }}
{{- end }}
{{- end }}
{{- if eq .Values.api.config.storageType "s3" }}
s3:
  endpoint: {{ .Values.api.s3.endpoint | quote }}
  bucket: {{ .Values.api.s3.bucket | quote }}
  region: {{ .Values.api.s3.region | quote }}
  tls_insecureskipverify: {{ .Values.api.s3.tlsInsecureSkipVerify }}
  path_style: {{ .Values.api.s3.pathStyle }}
  concurrency: {{ .Values.api.s3.concurrency | int }}
{{- end }}
{{- if .Values.api.oidc.enabled }}
oidc:
  enabled: true
  issuer_url: {{ .Values.api.oidc.issuerUrl | quote }}
  client_id: {{ .Values.api.oidc.clientId | quote }}
  redirect_url: {{ .Values.api.oidc.redirectUrl | quote }}
  scopes: {{ .Values.api.oidc.scopes | quote }}
  groups_claim: {{ .Values.api.oidc.groupsClaim | quote }}
  admin_groups: {{ .Values.api.oidc.adminGroups | quote }}
  editor_groups: {{ .Values.api.oidc.editorGroups | quote }}
  default_role: {{ .Values.api.oidc.defaultRole | quote }}
  post_login_redirect: {{ .Values.api.oidc.postLoginRedirect | quote }}
  {{- if .Values.api.oidc.endSessionUrl }}
  end_session_url: {{ .Values.api.oidc.endSessionUrl | quote }}
  {{- end }}
{{- end }}
{{- end }}

{{/*
Auto-compute VITE_API_URL.
Priority:
  1. Explicit ui.config.apiUrl — use as-is.
  2. Unified ingress enabled with a host — use relative "/api/v1" (same origin, no CORS).
  3. Fallback — in-cluster service URL.
*/}}
{{- define "alluredeck.ui.apiUrl" -}}
{{- if .Values.ui.config.apiUrl }}
{{- .Values.ui.config.apiUrl }}
{{- else if and .Values.ingress.enabled .Values.ingress.host }}
{{- print "/api/v1" }}
{{- else }}
{{- printf "http://%s.%s.svc.cluster.local:%d/api/v1" (include "alluredeck.api.fullname" .) .Release.Namespace (.Values.api.service.port | int) }}
{{- end }}
{{- end }}

{{/*
Image pull secrets combining global and component-level secrets.
*/}}
{{- define "alluredeck.imagePullSecrets" -}}
{{- $secrets := .Values.global.imagePullSecrets }}
{{- if $secrets }}
imagePullSecrets:
  {{- range $secrets }}
  - name: {{ . }}
  {{- end }}
{{- end }}
{{- end }}
