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
	"k8s.io/apimachinery/pkg/runtime"
)

// CheckRunPhase summarizes the lifecycle of a run.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Error
type CheckRunPhase string

const (
	// CheckRunPending means runner Jobs have not been created yet.
	CheckRunPending CheckRunPhase = "Pending"
	// CheckRunRunning means runner Jobs are executing.
	CheckRunRunning CheckRunPhase = "Running"
	// CheckRunSucceeded means all checks ran and passed.
	CheckRunSucceeded CheckRunPhase = "Succeeded"
	// CheckRunFailed means all checks ran but at least one did not pass.
	CheckRunFailed CheckRunPhase = "Failed"
	// CheckRunError means the run could not be executed (infrastructure
	// failure), as opposed to checks observing failures.
	CheckRunError CheckRunPhase = "Error"
)

// Condition types set on CheckRun.
const (
	// ConditionCompleted is True once the run reached a terminal phase.
	ConditionCompleted = "Completed"
	// ConditionDeadlineExceeded is True when the run hit its timeout.
	ConditionDeadlineExceeded = "DeadlineExceeded"
	// ConditionRunnerServiceAccountMissing is True when the runner
	// ServiceAccount does not exist in the run's namespace.
	ConditionRunnerServiceAccountMissing = "RunnerServiceAccountMissing"
)

// CheckRunSpec defines the desired state of CheckRun
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="CheckRun spec is immutable"
type CheckRunSpec struct {
	// suiteRef names the CheckSuite this run was created from.
	// Ad-hoc runs may omit it.
	// +optional
	SuiteRef *corev1.LocalObjectReference `json:"suiteRef,omitempty"`

	// suite is a full snapshot of the suite template taken when the run
	// was created, so later suite edits do not affect an in-flight run.
	// +required
	Suite CheckSuiteTemplate `json:"suite"`
}

// CheckResult is the outcome of one check from one runner pod.
type CheckResult struct {
	// +kubebuilder:validation:MaxLength=63
	// +required
	Name string `json:"name"`

	// passed is the verdict after expect is applied.
	// +required
	Passed bool `json:"passed"`

	// observed is the raw probe outcome, kept for debugging negative tests.
	// +required
	Observed ObservedOutcome `json:"observed"`

	// attempts actually used (>1 only when retries are configured).
	// +optional
	Attempts int32 `json:"attempts,omitempty"`

	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Message string `json:"message,omitempty"`

	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`
}

// PodResult is the complete result set reported by a single runner pod.
// Each pod applies exactly its own entry via server-side apply, so entries
// never conflict between pods.
type PodResult struct {
	// +kubebuilder:validation:MaxLength=253
	// +required
	PodName string `json:"podName"`

	// +kubebuilder:validation:MaxLength=253
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// +kubebuilder:validation:MaxItems=128
	// +listType=map
	// +listMapKey=name
	// +optional
	Checks []CheckResult `json:"checks,omitempty"`
}

// RunnerStatus groups pod results per runner.
type RunnerStatus struct {
	// +kubebuilder:validation:MaxLength=30
	// +required
	Name string `json:"name"`

	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=podName
	// +optional
	Pods []PodResult `json:"pods,omitempty"`
}

// RunSummary is a controller-owned aggregate over all reported results.
type RunSummary struct {
	// +optional
	Total int32 `json:"total,omitempty"`
	// +optional
	Passed int32 `json:"passed,omitempty"`
	// +optional
	Failed int32 `json:"failed,omitempty"`
}

// CheckRunStatus defines the observed state of CheckRun.
//
// Ownership is split between field managers: runner pods each apply only
// their own entry under runners[].pods[]; the controller applies everything
// else and never touches runners[].
type CheckRunStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Phase CheckRunPhase `json:"phase,omitempty"`

	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// runners holds results reported by runner pods via server-side apply.
	// +kubebuilder:validation:MaxItems=16
	// +listType=map
	// +listMapKey=name
	// +optional
	Runners []RunnerStatus `json:"runners,omitempty"`

	// +optional
	Summary *RunSummary `json:"summary,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ckr
// +kubebuilder:printcolumn:name="Suite",type=string,JSONPath=`.spec.suiteRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Passed",type=integer,JSONPath=`.status.summary.passed`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.summary.failed`
// +kubebuilder:printcolumn:name="Started",type=date,JSONPath=`.status.startTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CheckRun is the Schema for the checkruns API
type CheckRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CheckRun
	// +required
	Spec CheckRunSpec `json:"spec"`

	// status defines the observed state of CheckRun
	// +optional
	Status CheckRunStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CheckRunList contains a list of CheckRun
type CheckRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CheckRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &CheckRun{}, &CheckRunList{})
		return nil
	})
}
