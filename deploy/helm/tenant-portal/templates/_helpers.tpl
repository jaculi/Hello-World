{{/*
tenant-portal Helm chart helpers（仿 tenant-bc 模式）。
*/}}
{{- define "tenant-portal.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "tenant-portal.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tenant-portal.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: tenant-portal
{{- end }}

{{- define "tenant-portal.labels" -}}
{{ include "tenant-portal.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: jsl-platform
{{- end }}

{{- define "tenant-portal.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "tenant-portal.name" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
