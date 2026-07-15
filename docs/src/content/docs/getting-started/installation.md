---
title: Installation
description: Installing the VeriKube operator with Helm — namespaces, CRD lifecycle and chart options.
---

VeriKube is installed with Helm. The chart deploys the operator, its CRDs
and the RBAC needed for runner pods.

```bash
helm install verikube oci://ghcr.io/frauniki/charts/verikube \
  --namespace verikube-system --create-namespace \
  --set checkNamespaces='{payment,batch}'
```

:::note
Until the first release is published, install from a local checkout instead:
use `./charts/verikube` in place of the OCI reference and follow the image
build steps in the [Quickstart](/verikube/getting-started/quickstart/).
:::

## Check namespaces

`checkNamespaces` lists every namespace (in addition to the release
namespace) that will host CheckSuites. The chart provisions the runner
ServiceAccount and a RoleBinding in each of them.

A CheckSuite created in a namespace that is *not* listed fails fast: its
runs get a `RunnerServiceAccountMissing` condition telling you exactly
which ServiceAccount is missing where. To add a namespace later:

```bash
helm upgrade verikube oci://ghcr.io/frauniki/charts/verikube --reuse-values \
  --set checkNamespaces='{payment,batch,newteam}'
```

## CRD lifecycle

CRDs are installed as templated chart resources, not via Helm's special
`crds/` directory. Two consequences:

- **`helm upgrade` rolls schema changes.** You never have to apply CRD
  updates manually.
- **`helm uninstall` keeps your data.** CRDs are annotated with
  `helm.sh/resource-policy: keep` by default (`crds.keep=true`), so
  uninstalling the chart never deletes your CheckSuites or run history.

Set `crds.enabled=false` if the CRDs are managed by something else.

## Runner image

Runner Job pods use the operator image by default (`runnerImage: ""`).
Overriding `runnerImage` risks version skew: an older runner reports check
types it doesn't know as explicit failures (`unknown check type`) rather
than silently skipping them. Leave it empty unless you have a specific
reason.

## Metrics

The operator can expose check results as Prometheus metrics:

```bash
helm upgrade verikube oci://ghcr.io/frauniki/charts/verikube --reuse-values \
  --set metrics.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.reader.serviceAccountName=prometheus-kube-prometheus-prometheus \
  --set metrics.reader.namespace=monitoring
```

The endpoint is served over HTTPS with authentication and authorization;
`metrics.reader.*` binds a chart-provided ClusterRole to your Prometheus
ServiceAccount so it can scrape. See
[Observability](/verikube/guides/observability/) for the full setup.

## Security-related values

| Value | Default | Meaning |
|---|---|---|
| `allowLocalTargets` | `false` | Checks targeting loopback / link-local addresses (e.g. cloud metadata, `169.254.169.254`) are refused. Set `true` to allow. |
| `runnerServiceAccount` | `verikube-runner` | ServiceAccount name for runner pods, created in each check namespace. |

See the [security model](/verikube/security/) before granting CheckSuite
permissions.

## Chart values reference

The full set of options lives in the chart's
[values.yaml](https://github.com/frauniki/verikube/blob/main/charts/verikube/values.yaml),
each documented with a comment.
