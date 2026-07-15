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
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	Name string `json:"name"`

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

	// +optional
	TCP *TCPCheck `json:"tcp,omitempty"`

	// +optional
	HTTP *HTTPCheck `json:"http,omitempty"`

	// +optional
	GRPC *GRPCCheck `json:"grpc,omitempty"`

	// expect declares which raw observation makes the check pass.
	// Failure turns the check into a negative test.
	// +kubebuilder:default=Success
	// +optional
	Expect ExpectedOutcome `json:"expect,omitempty"`

	// +optional
	Retries *RetryPolicy `json:"retries,omitempty"`
}

// TopologySpread spreads runner pods across a topology domain. It is
// rendered as a maxSkew=1 topologySpreadConstraint on the runner Job.
type TopologySpread struct {
	// +kubebuilder:validation:MaxLength=316
	// +kubebuilder:default=`topology.kubernetes.io/zone`
	// +optional
	TopologyKey string `json:"topologyKey,omitempty"`

	// +kubebuilder:validation:Enum=ScheduleAnyway;DoNotSchedule
	// +kubebuilder:default=ScheduleAnyway
	// +optional
	WhenUnsatisfiable corev1.UnsatisfiableConstraintAction `json:"whenUnsatisfiable,omitempty"`
}

// RunnerSpec defines where checks execute from: a set of pods created with
// the given scheduling constraints.
type RunnerSpec struct {
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

	// +kubebuilder:validation:MaxProperties=16
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:MaxItems=16
	// +listType=atomic
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

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
