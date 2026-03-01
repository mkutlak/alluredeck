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
Create the name of the service account to use.
*/}}
{{- define "alluredeck.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "alluredeck.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
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
Auto-compute CORS allowed origins from ingress config.
Returns a comma-separated string of origins.
If api.config.corsAllowedOrigins is explicitly set, use that.
Otherwise, derive from ingress UI host.
*/}}
{{- define "alluredeck.api.corsOrigins" -}}
{{- if .Values.api.config.corsAllowedOrigins }}
{{- .Values.api.config.corsAllowedOrigins }}
{{- else if and .Values.ingress.enabled .Values.ingress.ui.host }}
  {{- $scheme := "http" }}
  {{- if .Values.ingress.ui.tls }}
    {{- $scheme = "https" }}
  {{- end }}
  {{- printf "%s://%s" $scheme .Values.ingress.ui.host }}
{{- end }}
{{- end }}

{{/*
Auto-compute VITE_API_URL from ingress config.
If ui.config.apiUrl is explicitly set, use that.
Otherwise, derive from ingress API host.
*/}}
{{- define "alluredeck.ui.apiUrl" -}}
{{- if .Values.ui.config.apiUrl }}
{{- .Values.ui.config.apiUrl }}
{{- else if and .Values.ingress.enabled .Values.ingress.api.host }}
  {{- $scheme := "http" }}
  {{- if .Values.ingress.api.tls }}
    {{- $scheme = "https" }}
  {{- end }}
  {{- printf "%s://%s/api/v1" $scheme .Values.ingress.api.host }}
{{- else }}
{{- printf "http://%s:%d/api/v1" (include "alluredeck.api.fullname" .) (.Values.api.service.port | int) }}
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
