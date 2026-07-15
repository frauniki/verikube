---
title: Observability
description: Exposing check results as Prometheus metrics and visualizing them in Grafana.
---

VeriKube's data flow keeps scraping simple: runner pods report results into
the CheckRun's `.status` (server-side apply), and the operator turns
completed runs into Prometheus metrics on its own `/metrics` endpoint.
Runner pods are short-lived Job pods and expose no metrics themselves.

```
runner pods ──SSA──▶ CheckRun .status ──▶ operator /metrics ──scrape──▶ Prometheus ──▶ Grafana
```

## Metrics

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `verikube_check_last_result` | gauge | `namespace,suite,check` | Verdict of the last completed run: 1 = passed on every pod, 0 = failed on at least one |
| `verikube_check_duration_seconds` | histogram | `namespace,suite,check,result` | Per-probe latency (1ms–32s buckets) |
| `verikube_check_result_total` | counter | `namespace,suite,check,result` | Cumulative verdicts (`result` = `pass`/`fail`) |
| `verikube_checkruns_total` | counter | `namespace,suite,phase` | Terminal runs (`Succeeded`/`Failed`/`Error`) |
| `verikube_checkrun_duration_seconds` | histogram | `namespace,suite` | Whole-run duration |
| `verikube_checkrun_last_completion_timestamp_seconds` | gauge | `namespace,suite` | When the suite last completed a run **with results** |

Semantics worth knowing:

- `namespace`/`suite` identify the CheckSuite; labels deliberately exclude
  runner and pod names to keep cardinality bounded by what you define.
- Runs that end in phase `Error` (runner Job failed, deadline exceeded,
  missing ServiceAccount) produce **no check results**, so the two gauges
  keep their previous values — pair the staleness gauge with
  `verikube_checkruns_total{phase="Error"}` to catch this.
- Deleting a CheckSuite removes its gauge series.

## Setup with kube-prometheus-stack

1. Install [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
   (any prometheus-operator deployment works):

   ```bash
   helm install monitoring prometheus-community/kube-prometheus-stack \
     --namespace monitoring --create-namespace
   ```

2. Enable verikube metrics. The metrics endpoint is served over HTTPS with
   authentication/authorization, so the scraping Prometheus needs `get` on
   the `/metrics` nonResourceURL — `metrics.reader.*` binds the
   chart-provided ClusterRole to your Prometheus ServiceAccount:

   ```bash
   helm upgrade verikube oci://ghcr.io/frauniki/charts/verikube --reuse-values \
     --set metrics.enabled=true \
     --set metrics.serviceMonitor.enabled=true \
     --set metrics.reader.serviceAccountName=monitoring-kube-prometheus-prometheus \
     --set metrics.reader.namespace=monitoring
   ```

   (Find the ServiceAccount name with
   `kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceAccountName}'`.)

3. Trigger a run and check the target:

   ```bash
   kubectl annotate checksuite <name> verikube.dev/run-now="$(date +%s)" --overwrite
   ```

   In the Prometheus UI, the verikube target should be **UP** and
   `verikube_check_last_result` should return series after the run
   completes.

The chart's ServiceMonitor sets `honorLabels: true` so the metric's own
`namespace` label (the CheckSuite's namespace) survives scraping. If you
write your own scrape config instead, do the same — with the default
`honor_labels: false` the label is renamed to `exported_namespace` and the
queries below break.

## Grafana panels

Useful starting queries (all filterable by `namespace`/`suite`):

```promql
# Status table / stat panel: what is red right now
verikube_check_last_result

# Failing check count
count(verikube_check_last_result == 0) OR on() vector(0)

# Failure rate per check
sum by (namespace, suite, check)
  (rate(verikube_check_result_total{result="fail"}[$__rate_interval]))

# Probe latency p50 / p95 / p99
histogram_quantile(0.95, sum by (check, le)
  (rate(verikube_check_duration_seconds_bucket[$__rate_interval])))

# Runs per hour by phase (stacked)
sum by (phase) (increase(verikube_checkruns_total[1h]))

# Seconds since the suite last produced results
time() - verikube_checkrun_last_completion_timestamp_seconds
```

For the status table, map value `1` → PASS (green) and `0` → FAIL (red)
with a value mapping and color-background cell display.

## Alert examples

```yaml
groups:
  - name: verikube
    rules:
      - alert: VerikubeCheckFailing
        expr: verikube_check_last_result == 0
        for: 10m
        labels: { severity: warning }
        annotations:
          summary: "Check {{ $labels.check }} in {{ $labels.namespace }}/{{ $labels.suite }} is failing"

      # Set the threshold to ~2x your longest schedule interval.
      - alert: VerikubeSuiteStale
        expr: time() - verikube_checkrun_last_completion_timestamp_seconds > 3600
        labels: { severity: warning }
        annotations:
          summary: "Suite {{ $labels.namespace }}/{{ $labels.suite }} has not completed a run with results in 1h"

      - alert: VerikubeRunErrors
        expr: increase(verikube_checkruns_total{phase="Error"}[1h]) > 0
        labels: { severity: warning }
        annotations:
          summary: "Runs for {{ $labels.namespace }}/{{ $labels.suite }} are erroring (no results produced)"
```
