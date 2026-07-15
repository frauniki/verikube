---
title: Security model
description: The trust boundary, runner pod privileges, local-target blocking and residual risks.
---

## The trust boundary is RBAC on CheckSuite/CheckRun create

Whoever can create a CheckSuite (or a CheckRun directly, for ad hoc runs) can probe arbitrary addresses from arbitrarily placed pods — **that is the tool's purpose**. (The one exception: local targets are blocked by default, see below.) There is no allowlist of targets inside the suite spec; the control is *who may create suites, and where*.

Grant the CRD roles per namespace, deliberately:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: verikube-editor
  namespace: payment
rules:
  - apiGroups: ["verikube.dev"]
    resources: ["checksuites", "checkruns"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## Runner pod privileges

Runner pods are deliberately boring:

- fixed image and command, no extra privileges,
- a ServiceAccount limited to `get checkruns` and `patch checkruns/status` — enough to report results, nothing more.

## Local targets are blocked by default

Checks targeting loopback or link-local addresses — including cloud metadata endpoints like `169.254.169.254` — are **refused by default**, as a guard against using checks for SSRF-style probing of the node or cloud control plane. Opt out with `allowLocalTargets: true` in the chart values if probing such targets is genuinely intended.

## Residual risk: status writes within check namespaces

Within a namespace listed in `checkNamespaces`, anything with pod-create rights can mount the runner ServiceAccount and patch any CheckRun's status there. VeriKube cannot prevent this (the ServiceAccount must exist for runners to report), but it detects it: the controller emits `ForeignResultEntry` warning events for result entries whose pod names don't match its own Jobs.

Treat pod-create rights in check namespaces as implying "can write check results", and scope `checkNamespaces` accordingly.

## Reporting

Found a security issue? Please report it privately via [GitHub security advisories](https://github.com/frauniki/verikube/security/advisories/new) rather than a public issue.
