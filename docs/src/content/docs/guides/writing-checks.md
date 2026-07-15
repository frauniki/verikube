---
title: Writing checks
description: TCP, HTTP and gRPC probes, retries, and negative tests with expect Failure.
---

A check is one entry in a suite's `checks` list: a name, exactly one probe
type (`tcp`, `http` or `grpc`), and optional behavior like retries and
expected outcome. A full example combining everything:

```yaml
apiVersion: verikube.dev/v1alpha1
kind: CheckSuite
metadata:
  name: payment-network
  namespace: payment
spec:
  schedule: "*/30 * * * *"     # UTC. Omit for manual-only suites
  runners:
    - name: payment-nodes
      replicas: 3
      nodeSelector: { payment-ng: "true" }
      topologySpread:
        topologyKey: topology.kubernetes.io/zone
    - name: batch-nodes
      nodeSelector: { batch-ng: "true" }
  checks:
    - name: db-reachable               # all runners
      tcp: { address: "db.internal:3306", timeout: 5s }
    - name: api-health                 # payment nodes only
      runners: [payment-nodes]
      http:
        url: "https://api.internal/health"
        headers:
          - { name: Host, value: api.example.com }
        expectedStatus: [200]
      retries: { attempts: 3, delay: 5s }
    - name: payments-grpc              # gRPC Health Checking Protocol
      grpc:
        address: "payments.internal:50051"
        service: payments.v1.Payments  # omit to query overall server health
    - name: external-blocked           # negative test: must NOT connect
      tcp: { address: "blocked.example.com:443" }
      expect: Failure
```

## TCP

A TCP check passes when a connection can be established:

```yaml
- name: db-reachable
  tcp:
    address: "db.internal:3306"   # host:port, no scheme
    timeout: 5s                   # dial timeout, default 1s
```

## HTTP

An HTTP check performs a request and verifies the response status:

```yaml
- name: api-health
  http:
    url: "https://api.internal/health"
    method: GET                   # default GET
    headers:
      - { name: Host, value: api.example.com }
    expectedStatus: [200, 204]    # default [200]
    timeout: 10s                  # whole request, default 30s
```

A `Host` header overrides the request host — useful when probing through a
load balancer whose routing depends on the hostname while the URL targets
the LB address directly.

## gRPC

A gRPC check uses the standard
[gRPC Health Checking Protocol](https://grpc.io/docs/guides/health-checking/)
(`grpc.health.v1.Health/Check`) and passes when the server reports
`SERVING`:

```yaml
- name: payments-grpc
  grpc:
    address: "payments.internal:50051"
    service: payments.v1.Payments   # omit to query overall server health
    timeout: 5s                     # default 5s
    tls:                            # omit for plaintext
      insecureSkipVerify: true
```

## Negative tests

`expect: Failure` inverts the verdict: the check **passes when the probe
fails**. This turns "the firewall should block this" from an assumption
into a continuously verified assertion:

```yaml
- name: metadata-blocked
  tcp: { address: "internal-only.example.com:443", timeout: 2s }
  expect: Failure
```

The raw probe outcome is preserved in the result (`observed:
Success|Failure`) so a failing negative test shows you that the connection
*succeeded* when it shouldn't have.

## Retries

`retries` re-runs a check whose observed outcome does not match the
expected one; the last attempt's result is reported, and the attempt count
is recorded in the result:

```yaml
- name: flaky-endpoint
  http: { url: "https://api.internal/health" }
  retries:
    attempts: 3   # total, including the first; max 10
    delay: 5s     # between attempts, default 1s
```

## Restricting where a check runs

By default every check runs from every runner. `checks[].runners` names a
subset:

```yaml
runners:
  - name: payment-nodes
    nodeSelector: { payment-ng: "true" }
  - name: batch-nodes
    nodeSelector: { batch-ng: "true" }
checks:
  - name: api-health
    runners: [payment-nodes]     # must reference names defined above
    http: { url: "https://api.internal/health" }
```

A check passes overall only if it passed on **every pod** that ran it —
with `replicas: 3`, one bad node means a failed check, which is exactly
the point of running from multiple places.

All fields, defaults and validation rules are documented in the
[API reference](/verikube/reference/api/).
