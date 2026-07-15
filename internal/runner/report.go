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

package runner

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

// BuildApplyDocument builds the minimal server-side-apply document a runner
// pod submits: nothing but its own entry under status.runners[].pods[].
//
// It must stay minimal on purpose. Applying a typed CheckRun struct would
// serialize every non-omitempty zero field (phase, summary, ...) and hand
// this pod's field manager ownership over fields the controller owns —
// exactly the conflict the two-level listType=map layout exists to avoid.
func BuildApplyDocument(namespace, name, runnerName string, pod verikubev1alpha1.PodResult) (*unstructured.Unstructured, error) {
	podMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": verikubev1alpha1.SchemeGroupVersion.String(),
		"kind":       "CheckRun",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"status": map[string]any{
			"runners": []any{
				map[string]any{
					"name": runnerName,
					"pods": []any{podMap},
				},
			},
		},
	}}, nil
}

// report applies the pod's results to the CheckRun status subresource with
// the pod name as field manager, retrying transient failures.
func (r *Runner) report(ctx context.Context, pod verikubev1alpha1.PodResult) error {
	doc, err := BuildApplyDocument(r.Config.Namespace, r.Config.CheckRunName, r.Config.RunnerName, pod)
	if err != nil {
		return err
	}

	backoff := wait.Backoff{
		Steps:    8,
		Duration: 250 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		Cap:      15 * time.Second,
	}
	return retry.OnError(backoff, func(error) bool { return true }, func() error {
		return r.Client.Status().Apply(ctx,
			client.ApplyConfigurationFromUnstructured(doc.DeepCopy()),
			client.FieldOwner(r.Config.PodName))
	})
}
