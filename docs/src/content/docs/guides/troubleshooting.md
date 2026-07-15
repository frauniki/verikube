---
title: Troubleshooting
description: Symptom-to-fix mapping for the failure modes you are most likely to hit in production.
---

## A run ends in `Error`

`Error` means the run's infrastructure broke — there is no verdict, as
opposed to `Failed`, where checks ran and something was unreachable. Look
at the run's conditions first:

```bash
kubectl get checkrun <name> -o jsonpath='{.status.conditions}' | jq
```

| Condition / cause | Meaning | Fix |
|---|---|---|
| `RunnerServiceAccountMissing` | The runner ServiceAccount does not exist in the suite's namespace | Add the namespace to the chart's `checkNamespaces` value and `helm upgrade` |
| `DeadlineExceeded` | The run exceeded `spec.timeout` (default 10m) | Raise `timeout`, or check why runner pods were slow/unschedulable (`kubectl describe job`) |
| Runner Job failed | Pods couldn't be scheduled or crashed | `kubectl describe job -n <ns>` — typically a `nodeSelector` matching no nodes, or missing tolerations for tainted nodes |

`Error` runs produce **no check results**, so the state gauges
(`verikube_check_last_result`) keep their previous values. Alert on
`verikube_checkruns_total{phase="Error"}` and staleness to catch this —
see [Observability](/verikube/guides/observability/).

## A check fails with `unknown check type`

The runner image is older than the installed CRDs: it received a check
type it doesn't know and reported it as an explicit failure rather than
silently skipping it. This happens when `runnerImage` is overridden in the
chart. Remove the override (empty means "use the operator image") or align
the versions.

## A suite never runs on schedule

- Schedules are evaluated in **UTC** — a `0 9 * * *` schedule fires at
  09:00 UTC, not local time.
- Check `spec.suspend` — a suspended suite skips scheduled runs (manual
  triggers still work).
- A missed tick older than `startingDeadline` (default 200s) is skipped by
  design; the suite fires again at the *next* tick rather than catching
  up.

## The run-now annotation does nothing

The trigger fires when the annotation **value changes**. Re-applying the
same value is a no-op (the last handled value is recorded in
`status.lastManualTrigger`). Use a timestamp:

```bash
kubectl annotate checksuite <name> \
  verikube.dev/run-now="$(date +%s)" --overwrite
```

## A check targeting a local address is refused

Checks targeting loopback or link-local addresses (e.g. cloud metadata,
`169.254.169.254`) are refused by default as an SSRF guard. If probing
such targets is intended, set `allowLocalTargets: true` in the chart
values — after reading the [security model](/verikube/security/).

## Metrics queries return `exported_namespace`

Your scrape config lacks `honor_labels: true`: Prometheus renamed the
metric's own `namespace` label (the CheckSuite's namespace) to
`exported_namespace` because it collides with the target's namespace
label. The chart's ServiceMonitor sets this automatically; hand-written
scrape configs must do the same. See
[Observability](/verikube/guides/observability/).

## `ForeignResultEntry` warning events

The controller noticed result entries in a CheckRun's status whose pod
names don't match its own Jobs — something else in the namespace wrote
into the status. Within a check namespace, anything with pod-create rights
can mount the runner ServiceAccount, so treat this as a signal to review
who can create pods there. Background in the
[security model](/verikube/security/).
