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
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
	"github.com/frauniki/verikube/internal/checker"
)

const testRunnerName = "runner-a"

func testRunner() *Runner {
	opts := checker.Options{AllowLocalTargets: true}
	return &Runner{
		Registry: checker.NewRegistry(checker.NewTCPChecker(opts), checker.NewHTTPChecker(opts)),
		Config:   Config{MaxConcurrentChecks: 4},
		Log:      logr.Discard(),
	}
}

func TestFilterChecks(t *testing.T) {
	checks := []verikubev1alpha1.CheckSpec{
		{Name: "common"},
		{Name: "for-a", Runners: []string{testRunnerName}},
		{Name: "for-b", Runners: []string{"runner-b"}},
		{Name: "for-ab", Runners: []string{testRunnerName, "runner-b"}},
	}

	got := FilterChecks(checks, testRunnerName)
	want := []string{"common", "for-a", "for-ab"}
	if len(got) != len(want) {
		t.Fatalf("expected %d checks, got %d", len(want), len(got))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("expected check %q at index %d, got %q", name, i, got[i].Name)
		}
	}
}

func TestExecuteCheckExpectFailurePasses(t *testing.T) {
	// Grab a free port then close it: the dial fails, which is expected.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	res := testRunner().executeCheck(context.Background(), verikubev1alpha1.CheckSpec{
		Name:   "negative",
		TCP:    &verikubev1alpha1.TCPCheck{Address: addr},
		Expect: verikubev1alpha1.ExpectFailure,
	})
	if !res.Passed {
		t.Fatalf("expected negative test to pass, message: %s", res.Message)
	}
	if res.Observed != verikubev1alpha1.ObservedFailure {
		t.Fatalf("expected raw observation Failure, got %s", res.Observed)
	}
}

func TestExecuteCheckExpectSuccessAgainstOpenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	res := testRunner().executeCheck(context.Background(), verikubev1alpha1.CheckSpec{
		Name: "positive",
		TCP:  &verikubev1alpha1.TCPCheck{Address: ln.Addr().String()},
	})
	if !res.Passed {
		t.Fatalf("expected pass, message: %s", res.Message)
	}
	if res.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", res.Attempts)
	}
}

func TestExecuteCheckRetriesUntilExpected(t *testing.T) {
	// The port stays closed, so with expect: Success every attempt is used.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	res := testRunner().executeCheck(context.Background(), verikubev1alpha1.CheckSpec{
		Name: "flaky",
		TCP:  &verikubev1alpha1.TCPCheck{Address: addr},
		Retries: &verikubev1alpha1.RetryPolicy{
			Attempts: 3,
			Delay:    &metav1.Duration{Duration: 10 * time.Millisecond},
		},
	})
	if res.Passed {
		t.Fatal("expected failure against a closed port")
	}
	if res.Attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", res.Attempts)
	}
}

func TestExecuteCheckDoesNotRetryWhenExpectationMet(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	res := testRunner().executeCheck(context.Background(), verikubev1alpha1.CheckSpec{
		Name: "no-retry-needed",
		TCP:  &verikubev1alpha1.TCPCheck{Address: ln.Addr().String()},
		Retries: &verikubev1alpha1.RetryPolicy{
			Attempts: 5,
			Delay:    &metav1.Duration{Duration: 10 * time.Millisecond},
		},
	})
	if !res.Passed {
		t.Fatalf("expected pass, message: %s", res.Message)
	}
	if res.Attempts != 1 {
		t.Fatalf("expected no retries once the expectation is met, got %d attempts", res.Attempts)
	}
}

func TestExecuteCheckUnknownType(t *testing.T) {
	r := testRunner()
	// A registry without the http checker simulates an old runner image.
	r.Registry = checker.NewRegistry(checker.NewTCPChecker(checker.Options{AllowLocalTargets: true}))

	res := r.executeCheck(context.Background(), verikubev1alpha1.CheckSpec{
		Name: "future-check",
		HTTP: &verikubev1alpha1.HTTPCheck{URL: "https://example.com"},
	})
	if res.Passed {
		t.Fatal("unknown check types must fail loud, not pass")
	}
	if res.Message != "unknown check type (runner image too old?)" {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestBuildApplyDocumentIsMinimal(t *testing.T) {
	now := metav1.Now()
	pod := verikubev1alpha1.PodResult{
		PodName:        "run-a-xyz",
		NodeName:       "node-1",
		StartTime:      &now,
		CompletionTime: &now,
		Checks: []verikubev1alpha1.CheckResult{
			{Name: "c1", Passed: true, Observed: verikubev1alpha1.ObservedSuccess},
		},
	}
	doc, err := BuildApplyDocument("ns", "run-a", testRunnerName, pod)
	if err != nil {
		t.Fatal(err)
	}

	status, ok := doc.Object["status"].(map[string]any)
	if !ok {
		t.Fatal("missing status")
	}
	// The apply document must never contain controller-owned status fields;
	// including them would transfer their field ownership to this pod.
	for _, forbidden := range []string{"phase", "summary", "conditions", "startTime", "completionTime", "observedGeneration"} {
		if _, exists := status[forbidden]; exists {
			t.Fatalf("apply document must not contain controller-owned field %q", forbidden)
		}
	}
	if _, exists := doc.Object["spec"]; exists {
		t.Fatal("apply document must not contain spec")
	}

	runners := status["runners"].([]any)
	if len(runners) != 1 {
		t.Fatalf("expected exactly one runner entry, got %d", len(runners))
	}
	entry := runners[0].(map[string]any)
	if entry["name"] != testRunnerName {
		t.Fatalf("unexpected runner name %v", entry["name"])
	}
	pods := entry["pods"].([]any)
	if len(pods) != 1 {
		t.Fatalf("expected exactly one pod entry, got %d", len(pods))
	}
	podEntry := pods[0].(map[string]any)
	if podEntry["podName"] != "run-a-xyz" {
		t.Fatalf("unexpected podName %v", podEntry["podName"])
	}
}
