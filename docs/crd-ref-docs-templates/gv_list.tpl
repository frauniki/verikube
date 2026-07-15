{{- define "gvList" -}}
{{- $groupVersions := . -}}
---
title: API Reference
editUrl: false
tableOfContents:
  maxHeadingLevel: 4
---

## Packages
{{- range $groupVersions }}
- {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
