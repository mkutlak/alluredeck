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
{{- if .Values.serviceAccount.api.create }}
{{- default (printf "%s-api" (include "alluredeck.fullname" .)) .Values.serviceAccount.api.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.api.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the UI service account to use.
*/}}
{{- define "alluredeck.ui.serviceAccountName" -}}
{{- if .Values.serviceAccount.ui.create }}
{{- default (printf "%s-ui" (include "alluredeck.fullname" .)) .Values.serviceAccount.ui.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.ui.name }}
{{- end }}
{{- end }}

{{/*
API Secret name — use existingSecret if set, otherwise generate name.
*/}}
{{- define "alluredeck.api.secretName" -}}
{{- if .Values.api.existingSecret }}
{{- .Values.api.existingSecret }}
{{- else }}
{{- include "alluredeck.api.fullname" . }}
{{- end }}
{{- end }}

{{/*
Render the full API config.yaml content for the ConfigMap.
Non-secret settings only; secrets are injected as env vars from the Secret.
CORS: uses api.config.corsAllowedOrigins list if set, otherwise auto-derives
a single origin from ui.ingress.host.
*/}}
{{- define "alluredeck.api.configYAML" -}}
port: {{ .Values.api.service.port | quote }}
dev_mode: {{ .Values.api.config.devMode }}
security_enabled: {{ .Values.api.security.enabled }}
log_level: {{ .Values.api.config.logLevel | quote }}
storage_type: {{ .Values.api.config.storageType | quote }}
check_results_secs: {{ .Values.api.config.checkResultsEverySeconds | quote }}
keep_history: {{ .Values.api.config.keepHistory }}
keep_history_latest: {{ .Values.api.config.keepHistoryLatest | int }}
api_response_less_verbose: {{ .Values.api.config.apiResponseLessVerbose }}
trust_forwarded_for: {{ .Values.api.config.trustXForwardedFor }}
make_viewer_endpoints_public: {{ .Values.api.config.makeViewerEndpointsPublic }}
projects_directory: {{ .Values.api.config.staticContentProjects | quote }}
database_path: {{ .Values.api.config.databasePath | quote }}
{{- $corsOrigins := .Values.api.config.corsAllowedOrigins }}
{{- if and (empty $corsOrigins) .Values.ui.ingress.enabled .Values.ui.ingress.host }}
{{- $scheme := ternary "https" "http" (not (empty .Values.ui.ingress.tls)) }}
{{- $corsOrigins = list (printf "%s://%s" $scheme .Values.ui.ingress.host) }}
{{- end }}
{{- if $corsOrigins }}
cors_allowed_origins:
{{- range $corsOrigins }}
  - {{ . | quote }}
{{- end }}
{{- end }}
{{- if eq .Values.api.config.storageType "s3" }}
s3_endpoint: {{ .Values.api.s3.endpoint | quote }}
s3_bucket: {{ .Values.api.s3.bucket | quote }}
s3_region: {{ .Values.api.s3.region | quote }}
s3_use_ssl: {{ .Values.api.s3.useSSL }}
s3_path_style: {{ .Values.api.s3.pathStyle }}
s3_concurrency: {{ .Values.api.s3.concurrency | int }}
{{- end }}
{{- end }}

{{/*
Auto-compute VITE_API_URL.
If ui.config.apiUrl is explicitly set, use that.
Otherwise, default to the internal API service name so traffic stays in-cluster.
*/}}
{{- define "alluredeck.ui.apiUrl" -}}
{{- if .Values.ui.config.apiUrl }}
{{- .Values.ui.config.apiUrl }}
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
