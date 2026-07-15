/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExpectedOutcome declares which raw observation makes a check pass.
// +kubebuilder:validation:Enum=Success;Failure
type ExpectedOutcome string

const (
	// ExpectSuccess passes the check when the probe succeeds (default).
	ExpectSuccess ExpectedOutcome = "Success"
	// ExpectFailure passes the check when the probe fails, e.g. verifying
	// that a security group blocks a connection (negative test).
	ExpectFailure ExpectedOutcome = "Failure"
)

// ObservedOutcome is the raw result of a probe, before Expect is applied.
// +kubebuilder:validation:Enum=Success;Failure
type ObservedOutcome string

const (
	ObservedSuccess ObservedOutcome = "Success"
	ObservedFailure ObservedOutcome = "Failure"
)

// TCPCheck probes a TCP endpoint by establishing a connection.
// +kubebuilder:validation:XValidation:rule="!self.address.startsWith('http://') && !self.address.startsWith('https://') && !self.address.startsWith('tcp://')",message="address must be host:port without a scheme"
type TCPCheck struct {
	// address is the endpoint to dial, in host:port form.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Address string `json:"address"`

	// timeout for the dial attempt. Defaults to 1s.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// HTTPHeader is a header to send with an HTTP check request.
type HTTPHeader struct {
	// name of the header.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	Name string `json:"name"`

	// value of the header.
	// +kubebuilder:validation:MaxLength=2048
	// +required
	Value string `json:"value"`
}

// HTTPCheck probes an HTTP(S) endpoint and verifies the response status.
type HTTPCheck struct {
	// url of the request. Must start with http:// or https://.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^https?://`
	// +required
	URL string `json:"url"`

	// method of the request. Defaults to GET.
	// +kubebuilder:validation:Enum=GET;HEAD;POST;PUT;PATCH;DELETE;OPTIONS
	// +kubebuilder:default=GET
	// +optional
	Method string `json:"method,omitempty"`

	// headers to send with the request. A "Host" header overrides the
	// request host (useful when probing through a load balancer).
	// +kubebuilder:validation:MaxItems=32
	// +listType=map
	// +listMapKey=name
	// +optional
	Headers []HTTPHeader `json:"headers,omitempty"`

	// timeout for the whole request. Defaults to 30s.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// expectedStatus lists acceptable response status codes. Defaults to [200].
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:items:Minimum=100
	// +kubebuilder:validation:items:Maximum=599
	// +listType=set
	// +optional
	ExpectedStatus []int32 `json:"expectedStatus,omitempty"`
}

// GRPCTLS configures TLS for a gRPC check connection.
type GRPCTLS struct {
	// insecureSkipVerify skips verification of the server certificate.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// GRPCCheck probes a gRPC server using the standard gRPC Health Checking
// Protocol (grpc.health.v1.Health/Check). The probe observes success when
// the server reports SERVING.
// +kubebuilder:validation:XValidation:rule="!self.address.startsWith('http://') && !self.address.startsWith('https://') && !self.address.startsWith('grpc://')",message="address must be host:port without a scheme"
type GRPCCheck struct {
	// address is the endpoint to dial, in host:port form.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Address string `json:"address"`

	// service is the health-check service name to query. Empty queries the
	// server's overall health.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	Service string `json:"service,omitempty"`

	// timeout for the whole check, connection establishment included.
	// Defaults to 5s.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// tls enables TLS for the connection. Omitted means plaintext.
	// +optional
	TLS *GRPCTLS `json:"tls,omitempty"`
}

// RetryPolicy retries a check whose observed outcome does not match the
// expected one. The result of the last attempt is reported.
type RetryPolicy struct {
	// attempts is the total number of attempts, including the first one.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=1
	// +optional
	Attempts int32 `json:"attempts,omitempty"`

	// delay between attempts. Defaults to 1s.
	// +optional
	Delay *metav1.Duration `json:"delay,omitempty"`
}

// CheckSpec defines a single network check. Exactly one probe type must be set.
// +kubebuilder:validation:XValidation:rule="[has(self.tcp), has(self.http), has(self.grpc)].filter(x, x).size() == 1",message="exactly one of tcp, http or grpc must be set"
type CheckSpec struct {
	// name identifies the check within the suite and in results and metrics.
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Name string `json:"name"`

	// runners restricts this check to the named runners.
	// Empty means the check runs from every runner in the suite.
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=30
	// +listType=set
	// +optional
	Runners []string `json:"runners,omitempty"`

	// tcp probes an endpoint by establishing a TCP connection.
	// +optional
	TCP *TCPCheck `json:"tcp,omitempty"`

	// http probes an HTTP(S) endpoint and verifies the response status.
	// +optional
	HTTP *HTTPCheck `json:"http,omitempty"`

	// grpc probes a server using the gRPC Health Checking Protocol.
	// +optional
	GRPC *GRPCCheck `json:"grpc,omitempty"`

	// expect declares which raw observation makes the check pass.
	// Failure turns the check into a negative test.
	// +kubebuilder:default=Success
	// +optional
	Expect ExpectedOutcome `json:"expect,omitempty"`

	// retries re-runs the check when its observed outcome does not match
	// the expected one.
	// +optional
	Retries *RetryPolicy `json:"retries,omitempty"`
}

// TopologySpread spreads runner pods across a topology domain. It is
// rendered as a maxSkew=1 topologySpreadConstraint on the runner Job.
type TopologySpread struct {
	// topologyKey is the node label defining the domains to spread across.
	// Defaults to topology.kubernetes.io/zone.
	// +kubebuilder:validation:MaxLength=316
	// +kubebuilder:default=`topology.kubernetes.io/zone`
	// +optional
	TopologyKey string `json:"topologyKey,omitempty"`

	// whenUnsatisfiable controls scheduling when the spread cannot be
	// satisfied. Defaults to ScheduleAnyway.
	// +kubebuilder:validation:Enum=ScheduleAnyway;DoNotSchedule
	// +kubebuilder:default=ScheduleAnyway
	// +optional
	WhenUnsatisfiable corev1.UnsatisfiableConstraintAction `json:"whenUnsatisfiable,omitempty"`
}

// RunnerSpec defines where checks execute from: a set of pods created with
// the given scheduling constraints.
type RunnerSpec struct {
	// name identifies the runner within the suite and in checks[].runners.
	// +kubebuilder:validation:MaxLength=30
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Name string `json:"name"`

	// replicas is the number of runner pods. Each pod executes the full
	// set of checks assigned to this runner.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// nodeSelector restricts runner pods to nodes with these labels.
	// +kubebuilder:validation:MaxProperties=16
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// tolerations let runner pods schedule onto tainted nodes.
	// +kubebuilder:validation:MaxItems=16
	// +listType=atomic
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// topologySpread spreads runner pods across a topology domain.
	// +optional
	TopologySpread *TopologySpread `json:"topologySpread,omitempty"`
}

// CheckSuiteTemplate is the executable part of a suite. It is snapshotted
// into each CheckRun at creation time.
// +kubebuilder:validation:XValidation:rule="self.checks.all(c, !has(c.runners) || c.runners.all(r, self.runners.exists(x, x.name == r)))",message="checks[].runners must reference names defined in runners[]"
type CheckSuiteTemplate struct {
	// runners define where the checks execute from.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +listType=map
	// +listMapKey=name
	// +required
	Runners []RunnerSpec `json:"runners"`

	// checks define what to probe. Each check runs from every runner
	// unless it names specific ones in its runners field.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=128
	// +listType=map
	// +listMapKey=name
	// +required
	Checks []CheckSpec `json:"checks"`

	// timeout is the deadline for the whole run. A run exceeding it is
	// marked Error with a DeadlineExceeded condition. Defaults to 10m.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}
