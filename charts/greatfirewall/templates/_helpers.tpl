{{- define "gfw.name" -}}
{{- printf "%s-greatfirewall" .Release.Name | trunc 48 | trimSuffix "-" -}}
{{- end -}}

{{- define "gfw.validate" -}}
{{- if not .Values.platform.ipv6Disabled -}}
{{- fail "Set platform.ipv6Disabled=true only after disabling IPv6 for workloads" -}}
{{- end -}}
{{- $_ := required "api.tokenSecret is required" .Values.api.tokenSecret -}}
{{- if .Values.mock.enabled -}}
{{- $_ := required "mock.tlsSecret is required" .Values.mock.tlsSecret -}}
{{- else -}}
{{- if not (hasPrefix "https://" .Values.api.url) -}}
{{- fail "api.url must start with https://" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gfw.apiURL" -}}
{{- if .Values.mock.enabled -}}
{{- printf "https://%s-mock.%s.svc:8443" (include "gfw.name" .) .Release.Namespace -}}
{{- else -}}
{{- .Values.api.url -}}
{{- end -}}
{{- end -}}
