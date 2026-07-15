---
title: Quickstart
description: Run your first network check on a local kind cluster in about five minutes.
---

This walkthrough builds VeriKube from source, installs it into a local
[kind](https://kind.sigs.k8s.io/) cluster and runs a first check suite.
You need Docker, kind, kubectl and Helm.

:::note
VeriKube has not published its first release yet, so the operator image and
Helm chart are built from source below. From v0.1.0 on you will be able to
skip the build and install directly:

```bash
helm install verikube oci://ghcr.io/frauniki/charts/verikube \
  --namespace verikube-system --create-namespace \
  --set checkNamespaces='{demo}'
```
:::

## 1. Build the image and create a cluster

```bash
git clone https://github.com/frauniki/verikube.git
cd verikube

make docker-build IMG=verikube:dev
kind create cluster --name verikube
kind load docker-image verikube:dev --name verikube
```

## 2. Install the operator

```bash
helm install verikube ./charts/verikube \
  --namespace verikube-system --create-namespace \
  --set image.repository=verikube \
  --set image.tag=dev \
  --set checkNamespaces='{demo}'
```

`checkNamespaces` lists every namespace that will host CheckSuites — the
chart provisions the runner ServiceAccount and RoleBinding there. We'll use
a `demo` namespace:

```bash
kubectl create namespace demo
```

## 3. Create a CheckSuite

Both checks below work in any fresh cluster: the first probes the
Kubernetes API service, the second asserts that a blackholed
[TEST-NET](https://datatracker.ietf.org/doc/html/rfc5737) address is
**not** reachable (a negative test).

```yaml
# suite.yaml
apiVersion: verikube.dev/v1alpha1
kind: CheckSuite
metadata:
  name: hello
  namespace: demo
spec:
  runners:
    - name: default
  checks:
    - name: kube-api-reachable
      tcp:
        address: "kubernetes.default.svc:443"
    - name: blackhole-blocked
      tcp:
        address: "192.0.2.1:443"
        timeout: 2s
      expect: Failure
```

```bash
kubectl apply -f suite.yaml
```

There is no `schedule`, so nothing runs yet — this suite is manual-only.

## 4. Trigger a run

```bash
kubectl annotate checksuite hello -n demo \
  verikube.dev/run-now="$(date +%s)" --overwrite
```

The operator creates a CheckRun, a runner Job executes both checks, and the
results land in the CheckRun's status:

```bash
$ kubectl get checkrun -n demo
NAME               SUITE   PHASE       PASSED   FAILED   STARTED   AGE
hello-1784112900   hello   Succeeded   2        0        10s       10s
```

Both checks passed — including the negative test, which passed *because*
the connection failed. Per-pod detail lives in the status:

```bash
kubectl get checkrun -n demo -o yaml | grep -A 12 'runners:'
```

## 5. Clean up

```bash
kind delete cluster --name verikube
```

## Next steps

- [Installation](/verikube/getting-started/installation/) — chart options,
  CRD lifecycle, metrics
- [Concepts](/verikube/concepts/) — how CheckSuite, CheckRun and runners fit
  together
- [Writing checks](/verikube/guides/writing-checks/) — TCP, HTTP, gRPC,
  retries and negative tests
- [Scheduling](/verikube/guides/scheduling/) — cron schedules and manual
  triggers
