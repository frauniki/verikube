---
title: API Reference
editUrl: false
tableOfContents:
  maxHeadingLevel: 4
---

## Packages
- [verikube.dev/v1alpha1](#verikubedevv1alpha1)


## verikube.dev/v1alpha1

Package v1alpha1 contains API Schema definitions for the  v1alpha1 API group.

### Resource Types
- [CheckRun](#checkrun)
- [CheckRunList](#checkrunlist)
- [CheckSuite](#checksuite)
- [CheckSuiteList](#checksuitelist)



#### CheckResult



CheckResult is the outcome of one check from one runner pod.



_Appears in:_
- [PodResult](#podresult)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the check this result belongs to. |  | MaxLength: 63 <br />Required: \{\} <br /> |
| `passed` _boolean_ | passed is the verdict after expect is applied. |  | Required: \{\} <br /> |
| `observed` _[ObservedOutcome](#observedoutcome)_ | observed is the raw probe outcome, kept for debugging negative tests. |  | Enum: [Success Failure] <br />Required: \{\} <br /> |
| `attempts` _integer_ | attempts actually used (>1 only when retries are configured). |  | Optional: \{\} <br /> |
| `message` _string_ | message is a human-readable explanation of the outcome, e.g. the<br />dial error for a failed TCP check. |  | MaxLength: 1024 <br />Optional: \{\} <br /> |
| `duration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | duration the probe took, last attempt only. |  | Optional: \{\} <br /> |


#### CheckRun



CheckRun is the Schema for the checkruns API



_Appears in:_
- [CheckRunList](#checkrunlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `verikube.dev/v1alpha1` | | |
| `kind` _string_ | `CheckRun` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[CheckRunSpec](#checkrunspec)_ | spec defines the desired state of CheckRun |  | Required: \{\} <br /> |
| `status` _[CheckRunStatus](#checkrunstatus)_ | status defines the observed state of CheckRun |  | Optional: \{\} <br /> |


#### CheckRunList



CheckRunList contains a list of CheckRun





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `verikube.dev/v1alpha1` | | |
| `kind` _string_ | `CheckRunList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[CheckRun](#checkrun) array_ |  |  |  |


#### CheckRunPhase

_Underlying type:_ _string_

CheckRunPhase summarizes the lifecycle of a run.

_Validation:_
- Enum: [Pending Running Succeeded Failed Error]

_Appears in:_
- [CheckRunStatus](#checkrunstatus)

| Field | Description |
| --- | --- |
| `Pending` | CheckRunPending means runner Jobs have not been created yet.<br /> |
| `Running` | CheckRunRunning means runner Jobs are executing.<br /> |
| `Succeeded` | CheckRunSucceeded means all checks ran and passed.<br /> |
| `Failed` | CheckRunFailed means all checks ran but at least one did not pass.<br /> |
| `Error` | CheckRunError means the run could not be executed (infrastructure<br />failure), as opposed to checks observing failures.<br /> |


#### CheckRunSpec



CheckRunSpec defines the desired state of CheckRun



_Appears in:_
- [CheckRun](#checkrun)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `suiteRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#localobjectreference-v1-core)_ | suiteRef names the CheckSuite this run was created from.<br />Ad-hoc runs may omit it. |  | Optional: \{\} <br /> |
| `suite` _[CheckSuiteTemplate](#checksuitetemplate)_ | suite is a full snapshot of the suite template taken when the run<br />was created, so later suite edits do not affect an in-flight run. |  | Required: \{\} <br /> |


#### CheckRunStatus



CheckRunStatus defines the observed state of CheckRun.

Ownership is split between field managers: runner pods each apply only
their own entry under runners[].pods[]; the controller applies everything
else and never touches runners[].



_Appears in:_
- [CheckRun](#checkrun)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | observedGeneration is the run generation most recently reconciled. |  | Optional: \{\} <br /> |
| `phase` _[CheckRunPhase](#checkrunphase)_ | phase summarizes the run lifecycle: Pending, Running, Succeeded,<br />Failed or Error. |  | Enum: [Pending Running Succeeded Failed Error] <br />Optional: \{\} <br /> |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | startTime is when the controller created the runner Jobs. |  | Optional: \{\} <br /> |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | completionTime is when the run reached a terminal phase. |  | Optional: \{\} <br /> |
| `runners` _[RunnerStatus](#runnerstatus) array_ | runners holds results reported by runner pods via server-side apply. |  | MaxItems: 16 <br />Optional: \{\} <br /> |
| `summary` _[RunSummary](#runsummary)_ | summary aggregates pass/fail counts over all reported results. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ | conditions describe the run's current state, e.g. Completed,<br />DeadlineExceeded or RunnerServiceAccountMissing. |  | Optional: \{\} <br /> |


#### CheckSpec



CheckSpec defines a single network check. Exactly one probe type must be set.



_Appears in:_
- [CheckSuiteSpec](#checksuitespec)
- [CheckSuiteTemplate](#checksuitetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name identifies the check within the suite and in results and metrics. |  | MaxLength: 63 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Required: \{\} <br /> |
| `runners` _string array_ | runners restricts this check to the named runners.<br />Empty means the check runs from every runner in the suite. |  | MaxItems: 16 <br />items:MaxLength: 30 <br />Optional: \{\} <br /> |
| `tcp` _[TCPCheck](#tcpcheck)_ | tcp probes an endpoint by establishing a TCP connection. |  | Optional: \{\} <br /> |
| `http` _[HTTPCheck](#httpcheck)_ | http probes an HTTP(S) endpoint and verifies the response status. |  | Optional: \{\} <br /> |
| `grpc` _[GRPCCheck](#grpccheck)_ | grpc probes a server using the gRPC Health Checking Protocol. |  | Optional: \{\} <br /> |
| `expect` _[ExpectedOutcome](#expectedoutcome)_ | expect declares which raw observation makes the check pass.<br />Failure turns the check into a negative test. | Success | Enum: [Success Failure] <br />Optional: \{\} <br /> |
| `retries` _[RetryPolicy](#retrypolicy)_ | retries re-runs the check when its observed outcome does not match<br />the expected one. |  | Optional: \{\} <br /> |


#### CheckSuite



CheckSuite is the Schema for the checksuites API



_Appears in:_
- [CheckSuiteList](#checksuitelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `verikube.dev/v1alpha1` | | |
| `kind` _string_ | `CheckSuite` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[CheckSuiteSpec](#checksuitespec)_ | spec defines the desired state of CheckSuite |  | Required: \{\} <br /> |
| `status` _[CheckSuiteStatus](#checksuitestatus)_ | status defines the observed state of CheckSuite |  | Optional: \{\} <br /> |


#### CheckSuiteList



CheckSuiteList contains a list of CheckSuite





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `verikube.dev/v1alpha1` | | |
| `kind` _string_ | `CheckSuiteList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[CheckSuite](#checksuite) array_ |  |  |  |


#### CheckSuiteSpec



CheckSuiteSpec defines the desired state of CheckSuite



_Appears in:_
- [CheckSuite](#checksuite)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `schedule` _string_ | schedule in standard cron format, evaluated in UTC.<br />When omitted the suite only runs on manual triggers (the<br />verikube.dev/run-now annotation). |  | MaxLength: 100 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `suspend` _boolean_ | suspend stops scheduled runs without deleting the suite.<br />Manual triggers still work while suspended. | false | Optional: \{\} <br /> |
| `concurrencyPolicy` _[ConcurrencyPolicy](#concurrencypolicy)_ | concurrencyPolicy describes how to treat a new run while a previous<br />one is still active: Allow, Forbid (skip the new run, default) or<br />Replace (delete the active run first). | Forbid | Enum: [Allow Forbid Replace] <br />Optional: \{\} <br /> |
| `startingDeadline` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | startingDeadline is how late a missed scheduled tick may still fire.<br />Older missed ticks are skipped, so unsuspending a suite or restarting<br />the operator does not fire stale catch-up runs. Defaults to 200s. |  | Optional: \{\} <br /> |
| `historyLimit` _[HistoryLimit](#historylimit)_ | historyLimit bounds how many finished CheckRuns are kept. |  | Optional: \{\} <br /> |
| `runners` _[RunnerSpec](#runnerspec) array_ | runners define where the checks execute from. |  | MaxItems: 16 <br />MinItems: 1 <br />Required: \{\} <br /> |
| `checks` _[CheckSpec](#checkspec) array_ | checks define what to probe. Each check runs from every runner<br />unless it names specific ones in its runners field. |  | MaxItems: 128 <br />MinItems: 1 <br />Required: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | timeout is the deadline for the whole run. A run exceeding it is<br />marked Error with a DeadlineExceeded condition. Defaults to 10m. |  | Optional: \{\} <br /> |


#### CheckSuiteStatus



CheckSuiteStatus defines the observed state of CheckSuite.



_Appears in:_
- [CheckSuite](#checksuite)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | observedGeneration is the suite generation most recently reconciled. |  | Optional: \{\} <br /> |
| `lastScheduleTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | lastScheduleTime is the scheduled (not actual) time of the most<br />recently created scheduled run. |  | Optional: \{\} <br /> |
| `lastManualTrigger` _string_ | lastManualTrigger echoes the last handled verikube.dev/run-now<br />annotation value, making manual triggers idempotent. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `active` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#objectreference-v1-core) array_ | active references CheckRuns that have not finished yet. |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#condition-v1-meta) array_ | conditions describe the suite's current state, e.g.<br />RunnerServiceAccountMissing. |  | Optional: \{\} <br /> |


#### CheckSuiteTemplate



CheckSuiteTemplate is the executable part of a suite. It is snapshotted
into each CheckRun at creation time.



_Appears in:_
- [CheckRunSpec](#checkrunspec)
- [CheckSuiteSpec](#checksuitespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `runners` _[RunnerSpec](#runnerspec) array_ | runners define where the checks execute from. |  | MaxItems: 16 <br />MinItems: 1 <br />Required: \{\} <br /> |
| `checks` _[CheckSpec](#checkspec) array_ | checks define what to probe. Each check runs from every runner<br />unless it names specific ones in its runners field. |  | MaxItems: 128 <br />MinItems: 1 <br />Required: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | timeout is the deadline for the whole run. A run exceeding it is<br />marked Error with a DeadlineExceeded condition. Defaults to 10m. |  | Optional: \{\} <br /> |


#### ConcurrencyPolicy

_Underlying type:_ _string_

ConcurrencyPolicy describes how to treat a scheduled run when a previous
run is still active.

_Validation:_
- Enum: [Allow Forbid Replace]

_Appears in:_
- [CheckSuiteSpec](#checksuitespec)

| Field | Description |
| --- | --- |
| `Allow` | AllowConcurrent lets runs overlap.<br /> |
| `Forbid` | ForbidConcurrent skips the new run while one is active (default).<br /> |
| `Replace` | ReplaceConcurrent deletes the active run and starts a new one.<br /> |


#### ExpectedOutcome

_Underlying type:_ _string_

ExpectedOutcome declares which raw observation makes a check pass.

_Validation:_
- Enum: [Success Failure]

_Appears in:_
- [CheckSpec](#checkspec)

| Field | Description |
| --- | --- |
| `Success` | ExpectSuccess passes the check when the probe succeeds (default).<br /> |
| `Failure` | ExpectFailure passes the check when the probe fails, e.g. verifying<br />that a security group blocks a connection (negative test).<br /> |


#### GRPCCheck



GRPCCheck probes a gRPC server using the standard gRPC Health Checking
Protocol (grpc.health.v1.Health/Check). The probe observes success when
the server reports SERVING.



_Appears in:_
- [CheckSpec](#checkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `address` _string_ | address is the endpoint to dial, in host:port form. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `service` _string_ | service is the health-check service name to query. Empty queries the<br />server's overall health. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | timeout for the whole check, connection establishment included.<br />Defaults to 5s. |  | Optional: \{\} <br /> |
| `tls` _[GRPCTLS](#grpctls)_ | tls enables TLS for the connection. Omitted means plaintext. |  | Optional: \{\} <br /> |


#### GRPCTLS



GRPCTLS configures TLS for a gRPC check connection.



_Appears in:_
- [GRPCCheck](#grpccheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `insecureSkipVerify` _boolean_ | insecureSkipVerify skips verification of the server certificate. |  | Optional: \{\} <br /> |


#### HTTPCheck



HTTPCheck probes an HTTP(S) endpoint and verifies the response status.



_Appears in:_
- [CheckSpec](#checkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url of the request. Must start with http:// or https://. |  | MaxLength: 2048 <br />MinLength: 1 <br />Pattern: `^https?://` <br />Required: \{\} <br /> |
| `method` _string_ | method of the request. Defaults to GET. | GET | Enum: [GET HEAD POST PUT PATCH DELETE OPTIONS] <br />Optional: \{\} <br /> |
| `headers` _[HTTPHeader](#httpheader) array_ | headers to send with the request. A "Host" header overrides the<br />request host (useful when probing through a load balancer). |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | timeout for the whole request. Defaults to 30s. |  | Optional: \{\} <br /> |
| `expectedStatus` _integer array_ | expectedStatus lists acceptable response status codes. Defaults to [200]. |  | MaxItems: 20 <br />items:Maximum: 599 <br />items:Minimum: 100 <br />Optional: \{\} <br /> |


#### HTTPHeader



HTTPHeader is a header to send with an HTTP check request.



_Appears in:_
- [HTTPCheck](#httpcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the header. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `value` _string_ | value of the header. |  | MaxLength: 2048 <br />Required: \{\} <br /> |


#### HistoryLimit



HistoryLimit bounds how many finished CheckRuns are kept per suite.



_Appears in:_
- [CheckSuiteSpec](#checksuitespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `successful` _integer_ | successful is how many Succeeded runs to keep. Defaults to 3. | 3 | Minimum: 0 <br />Optional: \{\} <br /> |
| `failed` _integer_ | failed is how many Failed runs to keep; it also covers runs that<br />ended in Error. Defaults to 5. | 5 | Minimum: 0 <br />Optional: \{\} <br /> |


#### ObservedOutcome

_Underlying type:_ _string_

ObservedOutcome is the raw result of a probe, before Expect is applied.

_Validation:_
- Enum: [Success Failure]

_Appears in:_
- [CheckResult](#checkresult)

| Field | Description |
| --- | --- |
| `Success` |  |
| `Failure` |  |


#### PodResult



PodResult is the complete result set reported by a single runner pod.
Each pod applies exactly its own entry via server-side apply, so entries
never conflict between pods.



_Appears in:_
- [RunnerStatus](#runnerstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podName` _string_ | podName of the reporting runner pod. |  | MaxLength: 253 <br />Required: \{\} <br /> |
| `nodeName` _string_ | nodeName the pod ran on. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | startTime of check execution in this pod. |  | Optional: \{\} <br /> |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#time-v1-meta)_ | completionTime of check execution in this pod. |  | Optional: \{\} <br /> |
| `checks` _[CheckResult](#checkresult) array_ | checks holds one result per check executed by this pod. |  | MaxItems: 128 <br />Optional: \{\} <br /> |


#### RetryPolicy



RetryPolicy retries a check whose observed outcome does not match the
expected one. The result of the last attempt is reported.



_Appears in:_
- [CheckSpec](#checkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `attempts` _integer_ | attempts is the total number of attempts, including the first one. | 1 | Maximum: 10 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `delay` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | delay between attempts. Defaults to 1s. |  | Optional: \{\} <br /> |


#### RunSummary



RunSummary is a controller-owned aggregate over all reported results.



_Appears in:_
- [CheckRunStatus](#checkrunstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `total` _integer_ | total number of reported check results across all pods. |  | Optional: \{\} <br /> |
| `passed` _integer_ | passed is the number of results whose verdict was pass. |  | Optional: \{\} <br /> |
| `failed` _integer_ | failed is the number of results whose verdict was fail. |  | Optional: \{\} <br /> |


#### RunnerSpec



RunnerSpec defines where checks execute from: a set of pods created with
the given scheduling constraints.



_Appears in:_
- [CheckSuiteSpec](#checksuitespec)
- [CheckSuiteTemplate](#checksuitetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name identifies the runner within the suite and in checks[].runners. |  | MaxLength: 30 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of runner pods. Each pod executes the full<br />set of checks assigned to this runner. | 1 | Maximum: 16 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `nodeSelector` _object (keys:string, values:string)_ | nodeSelector restricts runner pods to nodes with these labels. |  | MaxProperties: 16 <br />Optional: \{\} <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#toleration-v1-core) array_ | tolerations let runner pods schedule onto tainted nodes. |  | MaxItems: 16 <br />Optional: \{\} <br /> |
| `topologySpread` _[TopologySpread](#topologyspread)_ | topologySpread spreads runner pods across a topology domain. |  | Optional: \{\} <br /> |


#### RunnerStatus



RunnerStatus groups pod results per runner.



_Appears in:_
- [CheckRunStatus](#checkrunstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the runner as defined in the suite. |  | MaxLength: 30 <br />Required: \{\} <br /> |
| `pods` _[PodResult](#podresult) array_ | pods holds the result set reported by each runner pod. |  | MaxItems: 64 <br />Optional: \{\} <br /> |


#### TCPCheck



TCPCheck probes a TCP endpoint by establishing a connection.



_Appears in:_
- [CheckSpec](#checkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `address` _string_ | address is the endpoint to dial, in host:port form. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#duration-v1-meta)_ | timeout for the dial attempt. Defaults to 1s. |  | Optional: \{\} <br /> |


#### TopologySpread



TopologySpread spreads runner pods across a topology domain. It is
rendered as a maxSkew=1 topologySpreadConstraint on the runner Job.



_Appears in:_
- [RunnerSpec](#runnerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `topologyKey` _string_ | topologyKey is the node label defining the domains to spread across.<br />Defaults to topology.kubernetes.io/zone. | topology.kubernetes.io/zone | MaxLength: 316 <br />Optional: \{\} <br /> |
| `whenUnsatisfiable` _[UnsatisfiableConstraintAction](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.36/#unsatisfiableconstraintaction-v1-core)_ | whenUnsatisfiable controls scheduling when the spread cannot be<br />satisfied. Defaults to ScheduleAnyway. | ScheduleAnyway | Enum: [ScheduleAnyway DoNotSchedule] <br />Optional: \{\} <br /> |


