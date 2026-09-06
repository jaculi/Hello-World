{{/*
公共标签/选择器（全平台 Helm 规范，BP-04 §6）
*/}}
{{- define "tenant-bc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "tenant-bc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tenant-bc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: bc-p1
{{- end -}}

{{- define "tenant-bc.labels" -}}
app.kubernetes.io/name: {{ include "tenant-bc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: bc-p1
{{- end -}}
