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

// RunNowAnnotation triggers a manual run when its value changes, e.g.
// kubectl annotate checksuite NAME verikube.dev/run-now="$(date +%s)" --overwrite
const RunNowAnnotation = "verikube.dev/run-now"

// ConcurrencyPolicy describes how to treat a scheduled run when a previous
// run is still active.
// +kubebuilder:validation:Enum=Allow;Forbid;Replace
type ConcurrencyPolicy string

const (
	// AllowConcurrent lets runs overlap.
	AllowConcurrent ConcurrencyPolicy = "Allow"
	// ForbidConcurrent skips the new run while one is active (default).
	ForbidConcurrent ConcurrencyPolicy = "Forbid"
	// ReplaceConcurrent deletes the active run and starts a new one.
	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

// HistoryLimit bounds how many finished CheckRuns are kept per suite.
type HistoryLimit struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	// +optional
	Successful *int32 `json:"successful,omitempty"`

	// failed also covers runs that ended in Error.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=5
	// +optional
	Failed *int32 `json:"failed,omitempty"`
}

// CheckSuiteSpec defines the desired state of CheckSuite
type CheckSuiteSpec struct {
	// schedule in standard cron format, evaluated in UTC.
	// When omitted the suite only runs on manual triggers (the
	// verikube.dev/run-now annotation).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +optional
	Schedule *string `json:"schedule,omitempty"`

	// suspend stops scheduled runs without deleting the suite.
	// Manual triggers still work while suspended.
	// +kubebuilder:default=false
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// +kubebuilder:default=Forbid
	// +optional
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// startingDeadline is how late a missed scheduled tick may still fire.
	// Older missed ticks are skipped, so unsuspending a suite or restarting
	// the operator does not fire stale catch-up runs. Defaults to 200s.
	// +optional
	StartingDeadline *metav1.Duration `json:"startingDeadline,omitempty"`

	// +optional
	HistoryLimit *HistoryLimit `json:"historyLimit,omitempty"`

	CheckSuiteTemplate `json:",inline"`
}

// CheckSuiteStatus defines the observed state of CheckSuite.
type CheckSuiteStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastScheduleTime is the scheduled (not actual) time of the most
	// recently created scheduled run.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// lastManualTrigger echoes the last handled verikube.dev/run-now
	// annotation value, making manual triggers idempotent.
	// +kubebuilder:validation:MaxLength=253
	// +optional
	LastManualTrigger string `json:"lastManualTrigger,omitempty"`

	// active references CheckRuns that have not finished yet.
	// +kubebuilder:validation:MaxItems=32
	// +listType=atomic
	// +optional
	Active []corev1.ObjectReference `json:"active,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cks
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Suspend",type=boolean,JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="LastRun",type=date,JSONPath=`.status.lastScheduleTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CheckSuite is the Schema for the checksuites API
type CheckSuite struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CheckSuite
	// +required
	Spec CheckSuiteSpec `json:"spec"`

	// status defines the observed state of CheckSuite
	// +optional
	Status CheckSuiteStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CheckSuiteList contains a list of CheckSuite
type CheckSuiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CheckSuite `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &CheckSuite{}, &CheckSuiteList{})
		return nil
	})
}
