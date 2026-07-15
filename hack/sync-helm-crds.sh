#!/usr/bin/env bash
# Syncs controller-gen CRD output into the Helm chart as gated templates.
# config/crd/bases/ stays the single source of truth; run via
# `make helm-sync-crds` after changing API types.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
src_dir="${repo_root}/config/crd/bases"
dst_dir="${repo_root}/charts/verikube/templates/crds"

mkdir -p "${dst_dir}"
rm -f "${dst_dir}"/*.yaml

for src in "${src_dir}"/*.yaml; do
  name="$(basename "${src}")"
  dst="${dst_dir}/${name}"
  {
    echo '{{- if .Values.crds.enabled }}'
    # Inject the conditional keep policy right under the annotations key of
    # the CRD metadata (controller-gen always emits one).
    awk '
      { print }
      /^  annotations:$/ && !injected {
        print "    {{- if .Values.crds.keep }}"
        print "    helm.sh/resource-policy: keep"
        print "    {{- end }}"
        injected = 1
      }
    ' "${src}"
    echo '{{- end }}'
  } > "${dst}"
  echo "synced ${name}"
done
