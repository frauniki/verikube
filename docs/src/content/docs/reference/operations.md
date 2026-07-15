---
title: Operational notes
description: Timezone behavior, missed-tick handling, operator upgrades and version skew.
---

Facts worth knowing before running VeriKube in production.

## Schedules run in UTC

Cron expressions are standard 5-field cron, evaluated in **UTC**. There is no per-suite timezone setting; convert your intended local time when writing schedules.

## Missed ticks are dropped, not replayed

`startingDeadline` (default 200s) bounds how late a missed scheduled tick may still fire: the most recent missed tick still within the deadline fires (late), anything older is skipped. So:

- unsuspending a suite does not replay the suspended window — at most the final tick fires, and only if it was missed by less than the configured `startingDeadline`,
- restarting the operator does not fire a burst of runs for ticks missed while it was down.

Beyond that, the suite resumes at its next regular tick.

## Operator upgrades are safe with runs in flight

All state lives in the Kubernetes API: the CheckRun spec is an immutable snapshot, results arrive via server-side apply from runner pods, and runner Jobs are immutable once created. Replacing the operator pod mid-run loses nothing; the new pod picks up aggregation where the old one left off.

## Runner image version skew

Runner pods use the operator image by default. If you override `runnerImage` in the chart, an older runner that receives a check type it doesn't know reports it as an explicit failure (`unknown check type`) rather than silently skipping it — you'll see the failure instead of a false pass. Keep the override empty unless you have a specific reason.

## Resource footprint

The operator defaults to modest requests (50m CPU / 64Mi memory, 256Mi limit) — adjust via the chart's `resources` value. Runner pods are short-lived Job pods; their count is bounded by `runners[].replicas` (max 16 replicas × 16 runners per suite).

## History growth

Finished CheckRuns are garbage-collected per suite via `historyLimit` (default: 3 successful, 5 failed-or-error). Suites you never clean up cost etcd space proportional to those limits, not to runtime.
