---
title: Scheduling & triggering
description: Cron schedules, manual run-now triggers, suspend, concurrency policy and history limits.
---

A suite runs on a cron schedule, on manual triggers, or both.

## Cron schedules

`spec.schedule` is a standard 5-field cron expression, evaluated in **UTC**:

```yaml
spec:
  schedule: "*/30 * * * *"   # every 30 minutes
```

Omit `schedule` entirely for a manual-only suite.

Late or missed ticks (operator restart, suspended suite) are governed by `startingDeadline` (default 200s): a missed tick older than the deadline is skipped, so unsuspending a suite or restarting the operator does **not** fire a burst of stale catch-up runs.

## Manual triggers (run-now)

Changing the value of the `verikube.dev/run-now` annotation triggers one run — even while the suite is suspended:

```bash
kubectl annotate checksuite payment-network \
  verikube.dev/run-now="$(date +%s)" --overwrite
```

Any value works as long as it differs from the last handled one (the operator echoes it into `status.lastManualTrigger` to stay idempotent); a timestamp is a natural choice.

## Suspend

```yaml
spec:
  suspend: true
```

Stops scheduled runs without deleting the suite. Manual triggers still work while suspended — handy for debugging a suite you've paused.

## Concurrency policy

`concurrencyPolicy` decides what happens when a new run is due while a previous one is still active:

| Value | Behavior |
|---|---|
| `Forbid` (default) | Skip the new run while one is active |
| `Allow` | Let runs overlap |
| `Replace` | Delete the active run and start a new one |

With `Allow`, overlapping runs may complete out of order; the [state metrics](/verikube/guides/observability/) reflect the run that finished last.

## History limits

Finished CheckRuns are garbage-collected per suite:

```yaml
spec:
  historyLimit:
    successful: 3   # default 3
    failed: 5       # default 5; also covers Error runs
```

Keep `failed` higher than `successful` — failure history is what you'll be reading during an incident.

## Timeout

`spec.timeout` (default 10m) is the deadline for the whole run. A run exceeding it is marked `Error` with a `DeadlineExceeded` condition.
