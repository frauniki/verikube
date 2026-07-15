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

// Package runner executes the checks assigned to a single runner pod and
// reports the results into the owning CheckRun's status via server-side
// apply. A runner exits 0 whenever every check was executed and reported,
// regardless of check verdicts: a failed check is a successfully made
// observation, and retrying the pod would only re-run it for nothing (and
// would turn expect: Failure checks into retry loops). A non-zero exit
// means the runner itself could not do its job.
package runner

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
	"github.com/frauniki/verikube/internal/checker"
)

// Environment variables set on runner pods by the CheckRun controller.
const (
	EnvCheckRunName      = "VERIKUBE_CHECKRUN_NAME"
	EnvCheckRunNamespace = "VERIKUBE_CHECKRUN_NAMESPACE"
	EnvRunnerName        = "VERIKUBE_RUNNER_NAME"
	EnvPodName           = "POD_NAME"
	EnvNodeName          = "NODE_NAME"
)

const (
	defaultRetryDelay = 1 * time.Second
	// maxMessageLength matches the CRD's MaxLength on CheckResult.message.
	maxMessageLength = 1024
)

// Config identifies the runner pod's assignment.
type Config struct {
	CheckRunName        string
	Namespace           string
	RunnerName          string
	PodName             string
	NodeName            string
	MaxConcurrentChecks int
}

// ConfigFromEnv reads the assignment from the environment.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		CheckRunName: os.Getenv(EnvCheckRunName),
		Namespace:    os.Getenv(EnvCheckRunNamespace),
		RunnerName:   os.Getenv(EnvRunnerName),
		PodName:      os.Getenv(EnvPodName),
		NodeName:     os.Getenv(EnvNodeName),
	}
	for _, req := range []struct{ key, val string }{
		{EnvCheckRunName, cfg.CheckRunName},
		{EnvCheckRunNamespace, cfg.Namespace},
		{EnvRunnerName, cfg.RunnerName},
		{EnvPodName, cfg.PodName},
	} {
		if req.val == "" {
			return Config{}, fmt.Errorf("required environment variable %s is not set", req.key)
		}
	}
	return cfg, nil
}

// Runner executes checks and reports results.
type Runner struct {
	Client   client.Client
	Registry *checker.Registry
	Config   Config
	Log      logr.Logger
}

// Run fetches the CheckRun, executes the checks assigned to this runner,
// and applies the results. The returned error is a runner error, never a
// check verdict.
func (r *Runner) Run(ctx context.Context) error {
	startTime := metav1.Now()

	var run verikubev1alpha1.CheckRun
	key := types.NamespacedName{Namespace: r.Config.Namespace, Name: r.Config.CheckRunName}
	if err := r.Client.Get(ctx, key, &run); err != nil {
		return fmt.Errorf("failed to get CheckRun %s: %w", key, err)
	}

	checks := FilterChecks(run.Spec.Suite.Checks, r.Config.RunnerName)
	r.Log.Info("executing checks", "checkrun", key.String(), "runner", r.Config.RunnerName, "checks", len(checks))

	results := r.executeChecks(ctx, checks)
	completionTime := metav1.Now()

	pod := verikubev1alpha1.PodResult{
		PodName:        r.Config.PodName,
		NodeName:       r.Config.NodeName,
		StartTime:      &startTime,
		CompletionTime: &completionTime,
		Checks:         results,
	}
	if err := r.report(ctx, pod); err != nil {
		return fmt.Errorf("failed to report results: %w", err)
	}

	for _, res := range results {
		r.Log.Info("check finished", "check", res.Name, "passed", res.Passed,
			"observed", res.Observed, "attempts", res.Attempts, "message", res.Message)
	}
	return nil
}

// FilterChecks returns the checks assigned to the named runner: checks with
// no runner restriction plus checks that list the runner explicitly.
func FilterChecks(checks []verikubev1alpha1.CheckSpec, runnerName string) []verikubev1alpha1.CheckSpec {
	var out []verikubev1alpha1.CheckSpec
	for _, c := range checks {
		if len(c.Runners) == 0 || slices.Contains(c.Runners, runnerName) {
			out = append(out, c)
		}
	}
	return out
}

func (r *Runner) executeChecks(ctx context.Context, checks []verikubev1alpha1.CheckSpec) []verikubev1alpha1.CheckResult {
	results := make([]verikubev1alpha1.CheckResult, len(checks))

	limit := r.Config.MaxConcurrentChecks
	if limit <= 0 {
		limit = 8
	}
	g := new(errgroup.Group)
	g.SetLimit(limit)
	for i, c := range checks {
		g.Go(func() error {
			results[i] = r.executeCheck(ctx, c)
			return nil
		})
	}
	_ = g.Wait()
	return results
}

// executeCheck runs one check, applying retries and expect inversion.
func (r *Runner) executeCheck(ctx context.Context, spec verikubev1alpha1.CheckSpec) verikubev1alpha1.CheckResult {
	expected := spec.Expect
	if expected == "" {
		expected = verikubev1alpha1.ExpectSuccess
	}

	chk, ok := r.Registry.ForSpec(spec)
	if !ok {
		// Never skip silently: an older runner image seeing a newer check
		// type must surface it as a failure, not vanish from the results.
		return verikubev1alpha1.CheckResult{
			Name:     spec.Name,
			Passed:   false,
			Observed: verikubev1alpha1.ObservedFailure,
			Attempts: 1,
			Message:  "unknown check type (runner image too old?)",
		}
	}

	attempts := int32(1)
	delay := defaultRetryDelay
	if spec.Retries != nil {
		if spec.Retries.Attempts > 0 {
			attempts = spec.Retries.Attempts
		}
		if spec.Retries.Delay != nil {
			delay = spec.Retries.Delay.Duration
		}
	}

	var res checker.Result
	var observed verikubev1alpha1.ObservedOutcome
	attempt := int32(0)
	for {
		attempt++
		res = chk.Check(ctx, spec)
		observed = verikubev1alpha1.ObservedFailure
		if res.Observed {
			observed = verikubev1alpha1.ObservedSuccess
		}
		// Retry only while the observation does not match the expectation.
		if string(observed) == string(expected) || attempt >= attempts {
			break
		}
		select {
		case <-ctx.Done():
			attempt = attempts // stop retrying, keep the last observation
		case <-time.After(delay):
		}
		if attempt >= attempts {
			break
		}
	}

	message := res.Message
	if len(message) > maxMessageLength {
		message = message[:maxMessageLength]
	}
	return verikubev1alpha1.CheckResult{
		Name:     spec.Name,
		Passed:   string(observed) == string(expected),
		Observed: observed,
		Attempts: attempt,
		Message:  message,
		Duration: &metav1.Duration{Duration: res.Duration.Round(time.Millisecond)},
	}
}
