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

package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	testclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	verikubedevv1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
	"github.com/frauniki/verikube/internal/runner"
)

const (
	runnerImage       = "example.com/verikube:test"
	defaultRunnerName = "default"
)

func newCheckRunReconciler(clk *testclock.FakePassiveClock) (*CheckRunReconciler, *events.FakeRecorder) {
	recorder := events.NewFakeRecorder(100)
	r := &CheckRunReconciler{
		Client:      k8sClient,
		Scheme:      k8sClient.Scheme(),
		Recorder:    recorder,
		RunnerImage: runnerImage,
	}
	if clk != nil {
		r.Clock = clk
	}
	return r, recorder
}

func createNamespace() string {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "verikube-test-"}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns.Name
}

func createRunnerServiceAccount(namespace string) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      DefaultRunnerServiceAccount,
		Namespace: namespace,
	}}
	Expect(k8sClient.Create(ctx, sa)).To(Succeed())
}

func reconcileRun(r *CheckRunReconciler, run *verikubedevv1alpha1.CheckRun) ctrl.Result {
	res, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: run.Name, Namespace: run.Namespace},
	})
	Expect(err).NotTo(HaveOccurred())
	return res
}

func getRun(run *verikubedevv1alpha1.CheckRun) *verikubedevv1alpha1.CheckRun {
	out := &verikubedevv1alpha1.CheckRun{}
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(run), out)).To(Succeed())
	return out
}

// reportAsPod simulates a runner pod applying its results, using the same
// code path the real runner uses.
func reportAsPod(run *verikubedevv1alpha1.CheckRun, runnerName, podName string, results []verikubedevv1alpha1.CheckResult) {
	now := metav1.Now()
	pod := verikubedevv1alpha1.PodResult{
		PodName:        podName,
		NodeName:       "test-node",
		StartTime:      &now,
		CompletionTime: &now,
		Checks:         results,
	}
	doc, err := runner.BuildApplyDocument(run.Namespace, run.Name, runnerName, pod)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Status().Apply(ctx,
		client.ApplyConfigurationFromUnstructured(doc),
		client.FieldOwner(podName))).To(Succeed())
}

// markJobSucceeded updates the Job status the way the Job controller would
// on success (envtest runs no Job controller).
func markJobSucceeded(namespace, name string, completions int32) {
	job := &batchv1.Job{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, job)).To(Succeed())
	now := metav1.Now()
	if job.Status.StartTime == nil {
		job.Status.StartTime = &now
	}
	job.Status.Succeeded = completions
	job.Status.Conditions = append(job.Status.Conditions,
		batchv1.JobCondition{Type: batchv1.JobSuccessCriteriaMet, Status: corev1.ConditionTrue, LastTransitionTime: now},
		batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: now},
	)
	job.Status.CompletionTime = &now
	Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
}

func markJobFailed(namespace, name, message string) {
	job := &batchv1.Job{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, job)).To(Succeed())
	now := metav1.Now()
	if job.Status.StartTime == nil {
		job.Status.StartTime = &now
	}
	job.Status.Conditions = append(job.Status.Conditions,
		batchv1.JobCondition{Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue, LastTransitionTime: now, Message: message},
		batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: now, Message: message},
	)
	Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
}

func passResult(name string) verikubedevv1alpha1.CheckResult {
	return verikubedevv1alpha1.CheckResult{
		Name: name, Passed: true, Observed: verikubedevv1alpha1.ObservedSuccess, Attempts: 1,
	}
}

func failResult(name string) verikubedevv1alpha1.CheckResult {
	return verikubedevv1alpha1.CheckResult{
		Name: name, Passed: false, Observed: verikubedevv1alpha1.ObservedFailure, Attempts: 1,
		Message: "connection refused",
	}
}

var _ = Describe("CheckRun Controller", func() {
	newRun := func(namespace string, mutate func(*verikubedevv1alpha1.CheckRun)) *verikubedevv1alpha1.CheckRun {
		run := &verikubedevv1alpha1.CheckRun{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "run-", Namespace: namespace},
			Spec: verikubedevv1alpha1.CheckRunSpec{
				SuiteRef: &corev1.LocalObjectReference{Name: "test-suite"},
				Suite:    validTemplate(),
			},
		}
		if mutate != nil {
			mutate(run)
		}
		Expect(k8sClient.Create(ctx, run)).To(Succeed())
		return run
	}

	It("creates one Job per runner with the declared scheduling constraints", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, _ := newCheckRunReconciler(nil)

		run := newRun(ns, func(run *verikubedevv1alpha1.CheckRun) {
			run.Spec.Suite.Runners = []verikubedevv1alpha1.RunnerSpec{
				{
					Name:         "spread",
					Replicas:     ptr.To(int32(3)),
					NodeSelector: map[string]string{"payment-ng": "true"},
					Tolerations: []corev1.Toleration{
						{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "payment", Effect: corev1.TaintEffectNoSchedule},
					},
					TopologySpread: &verikubedevv1alpha1.TopologySpread{
						WhenUnsatisfiable: corev1.DoNotSchedule,
					},
				},
				{Name: "plain"},
			}
			run.Spec.Suite.Timeout = &metav1.Duration{Duration: 5 * time.Minute}
		})
		reconcileRun(r, run)

		spreadJob := &batchv1.Job{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: jobName(run.Name, "spread")}, spreadJob)).To(Succeed())
		Expect(*spreadJob.Spec.Parallelism).To(Equal(int32(3)))
		Expect(*spreadJob.Spec.Completions).To(Equal(int32(3)))
		Expect(*spreadJob.Spec.BackoffLimit).To(Equal(int32(2)))
		Expect(*spreadJob.Spec.ActiveDeadlineSeconds).To(Equal(int64(300)))
		Expect(spreadJob.OwnerReferences).To(HaveLen(1))
		Expect(spreadJob.OwnerReferences[0].Name).To(Equal(run.Name))

		podSpec := spreadJob.Spec.Template.Spec
		Expect(podSpec.NodeSelector).To(HaveKeyWithValue("payment-ng", "true"))
		Expect(podSpec.Tolerations).To(HaveLen(1))
		Expect(podSpec.ServiceAccountName).To(Equal(DefaultRunnerServiceAccount))
		Expect(podSpec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
		Expect(podSpec.TopologySpreadConstraints).To(HaveLen(1))
		spreadConstraint := podSpec.TopologySpreadConstraints[0]
		Expect(spreadConstraint.TopologyKey).To(Equal("topology.kubernetes.io/zone"))
		Expect(spreadConstraint.WhenUnsatisfiable).To(Equal(corev1.DoNotSchedule))
		Expect(spreadConstraint.MaxSkew).To(Equal(int32(1)))

		container := podSpec.Containers[0]
		Expect(container.Image).To(Equal(runnerImage))
		Expect(container.Args).To(Equal([]string{runnerSubcommand}))
		envNames := map[string]bool{}
		for _, e := range container.Env {
			envNames[e.Name] = true
		}
		for _, want := range []string{
			runner.EnvCheckRunName, runner.EnvCheckRunNamespace, runner.EnvRunnerName,
			runner.EnvPodName, runner.EnvNodeName,
		} {
			Expect(envNames).To(HaveKey(want), "missing env %s", want)
		}

		plainJob := &batchv1.Job{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: jobName(run.Name, "plain")}, plainJob)).To(Succeed())
		Expect(*plainJob.Spec.Parallelism).To(Equal(int32(1)))
		Expect(plainJob.Spec.Template.Spec.TopologySpreadConstraints).To(BeEmpty())

		updated := getRun(run)
		Expect(updated.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunRunning))
		Expect(updated.Status.StartTime).NotTo(BeNil())
	})

	It("fails fast with a condition when the runner ServiceAccount is missing", func() {
		ns := createNamespace() // deliberately no ServiceAccount
		r, recorder := newCheckRunReconciler(nil)

		run := newRun(ns, nil)
		reconcileRun(r, run)

		updated := getRun(run)
		Expect(updated.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunError))
		cond := meta.FindStatusCondition(updated.Status.Conditions, verikubedevv1alpha1.ConditionRunnerServiceAccountMissing)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Message).To(ContainSubstring("checkNamespaces"))
		Eventually(recorder.Events).Should(Receive(ContainSubstring("RunnerServiceAccountMissing")))

		// No Jobs must have been created.
		jobs := &batchv1.JobList{}
		Expect(k8sClient.List(ctx, jobs, client.InNamespace(ns))).To(Succeed())
		Expect(jobs.Items).To(BeEmpty())
	})

	It("keeps entries from concurrent runner pods and controller fields separate (SSA ownership)", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, _ := newCheckRunReconciler(nil)

		run := newRun(ns, func(run *verikubedevv1alpha1.CheckRun) {
			run.Spec.Suite.Runners = []verikubedevv1alpha1.RunnerSpec{
				{Name: defaultRunnerName, Replicas: ptr.To(int32(2))},
			}
		})
		reconcileRun(r, run) // creates Job, applies Running status

		job := jobName(run.Name, defaultRunnerName)
		podA, podB := job+"-aaaaa", job+"-bbbbb"
		reportAsPod(run, defaultRunnerName, podA, []verikubedevv1alpha1.CheckResult{passResult("example-tcp")})
		reportAsPod(run, defaultRunnerName, podB, []verikubedevv1alpha1.CheckResult{passResult("example-tcp")})

		updated := getRun(run)
		Expect(updated.Status.Runners).To(HaveLen(1))
		Expect(updated.Status.Runners[0].Pods).To(HaveLen(2), "both pods' SSA entries must coexist")
		Expect(updated.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunRunning),
			"controller-owned fields must survive runner patches")

		// managedFields: each pod's manager owns only its own entry, and
		// the controller manager owns nothing under runners.
		for _, mf := range updated.ManagedFields {
			if mf.Subresource != "status" || mf.FieldsV1 == nil {
				continue
			}
			var fields map[string]any
			Expect(json.Unmarshal(mf.FieldsV1.GetRawBytes(), &fields)).To(Succeed())
			status, _ := fields["f:status"].(map[string]any)
			if status == nil {
				continue
			}
			_, ownsRunners := status["f:runners"]
			switch mf.Manager {
			case podA, podB:
				Expect(ownsRunners).To(BeTrue(), "runner pod manager %s must own its entry", mf.Manager)
			case checkRunFieldManager:
				Expect(ownsRunners).To(BeFalse(), "controller must not own any key under runners")
			}
		}

		markJobSucceeded(ns, job, 2)
		reconcileRun(r, run)

		final := getRun(run)
		Expect(final.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunSucceeded))
		Expect(final.Status.Summary).NotTo(BeNil())
		Expect(final.Status.Summary.Total).To(Equal(int32(1)))
		Expect(final.Status.Summary.Passed).To(Equal(int32(1)))
		Expect(final.Status.Runners[0].Pods).To(HaveLen(2), "runner entries must survive the final controller apply")
		Expect(final.Status.CompletionTime).NotTo(BeNil())
		cond := meta.FindStatusCondition(final.Status.Conditions, verikubedevv1alpha1.ConditionCompleted)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("succeeds with more entries than replicas (retried pod that reported before dying)", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, _ := newCheckRunReconciler(nil)

		run := newRun(ns, func(run *verikubedevv1alpha1.CheckRun) {
			run.Spec.Suite.Runners = []verikubedevv1alpha1.RunnerSpec{
				{Name: defaultRunnerName, Replicas: ptr.To(int32(3))},
			}
		})
		reconcileRun(r, run)

		job := jobName(run.Name, defaultRunnerName)
		for _, suffix := range []string{"-a1", "-a2", "-a3", "-a4"} { // 4 entries, replicas=3
			reportAsPod(run, defaultRunnerName, job+suffix, []verikubedevv1alpha1.CheckResult{passResult("example-tcp")})
		}
		markJobSucceeded(ns, job, 3)
		reconcileRun(r, run)

		final := getRun(run)
		Expect(final.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunSucceeded),
			"len(pods) >= replicas must count as complete, == would wrongly error")
	})

	It("fails the run when any pod reports a failed check", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, recorder := newCheckRunReconciler(nil)

		run := newRun(ns, func(run *verikubedevv1alpha1.CheckRun) {
			run.Spec.Suite.Runners = []verikubedevv1alpha1.RunnerSpec{
				{Name: defaultRunnerName, Replicas: ptr.To(int32(2))},
			}
		})
		reconcileRun(r, run)

		job := jobName(run.Name, defaultRunnerName)
		reportAsPod(run, defaultRunnerName, job+"-ok", []verikubedevv1alpha1.CheckResult{passResult("example-tcp")})
		reportAsPod(run, defaultRunnerName, job+"-ng", []verikubedevv1alpha1.CheckResult{failResult("example-tcp")})
		markJobSucceeded(ns, job, 2)
		reconcileRun(r, run)

		final := getRun(run)
		Expect(final.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunFailed))
		Expect(final.Status.Summary.Failed).To(Equal(int32(1)))
		Eventually(recorder.Events).Should(Receive(ContainSubstring("CheckFailed")))
	})

	It("waits (not errors) when Jobs finished but results have not landed yet", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, _ := newCheckRunReconciler(nil)

		run := newRun(ns, nil)
		reconcileRun(r, run)

		markJobSucceeded(ns, jobName(run.Name, defaultRunnerName), 1)
		res := reconcileRun(r, run) // job done, no entries yet

		updated := getRun(run)
		Expect(updated.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunRunning),
			"missing entries must not flip the run to Error on a timer")
		Expect(res.RequeueAfter).To(BeNumerically(">", 0), "must keep the deadline requeue armed")

		// Once the (late) result lands, the run completes normally.
		reportAsPod(run, defaultRunnerName, jobName(run.Name, defaultRunnerName)+"-late", []verikubedevv1alpha1.CheckResult{passResult("example-tcp")})
		reconcileRun(r, run)
		Expect(getRun(run).Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunSucceeded))
	})

	It("errors the run when a runner Job fails", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, recorder := newCheckRunReconciler(nil)

		run := newRun(ns, nil)
		reconcileRun(r, run)

		markJobFailed(ns, jobName(run.Name, defaultRunnerName), "BackoffLimitExceeded")
		reconcileRun(r, run)

		final := getRun(run)
		Expect(final.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunError))
		Eventually(recorder.Events).Should(Receive(ContainSubstring("RunnerError")))
	})

	It("errors with DeadlineExceeded when the run outlives its timeout", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		clk := testclock.NewFakePassiveClock(time.Now())
		r, recorder := newCheckRunReconciler(clk)

		run := newRun(ns, func(run *verikubedevv1alpha1.CheckRun) {
			run.Spec.Suite.Timeout = &metav1.Duration{Duration: 30 * time.Second}
		})
		reconcileRun(r, run) // starts the run

		clk.SetTime(clk.Now().Add(31 * time.Second))
		reconcileRun(r, run)

		final := getRun(run)
		Expect(final.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunError))
		cond := meta.FindStatusCondition(final.Status.Conditions, verikubedevv1alpha1.ConditionDeadlineExceeded)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Eventually(recorder.Events).Should(Receive(ContainSubstring("DeadlineExceeded")))
	})

	It("aggregates per-runner effective checks (checks[].runners filtering)", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, _ := newCheckRunReconciler(nil)

		run := newRun(ns, func(run *verikubedevv1alpha1.CheckRun) {
			run.Spec.Suite.Runners = []verikubedevv1alpha1.RunnerSpec{
				{Name: "runner-a"}, {Name: "runner-b"},
			}
			run.Spec.Suite.Checks = []verikubedevv1alpha1.CheckSpec{
				{Name: "common", TCP: &verikubedevv1alpha1.TCPCheck{Address: "db:3306"}},
				{Name: "only-a", Runners: []string{"runner-a"}, TCP: &verikubedevv1alpha1.TCPCheck{Address: "a:80"}},
			}
		})
		reconcileRun(r, run)

		jobA, jobB := jobName(run.Name, "runner-a"), jobName(run.Name, "runner-b")
		// runner-a executes common + only-a; runner-b executes common only.
		reportAsPod(run, "runner-a", jobA+"-x", []verikubedevv1alpha1.CheckResult{passResult("common"), passResult("only-a")})
		reportAsPod(run, "runner-b", jobB+"-x", []verikubedevv1alpha1.CheckResult{passResult("common")})
		markJobSucceeded(ns, jobA, 1)
		markJobSucceeded(ns, jobB, 1)
		reconcileRun(r, run)

		final := getRun(run)
		Expect(final.Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunSucceeded))
		// total = runner-a(common, only-a) + runner-b(common) = 3
		Expect(final.Status.Summary.Total).To(Equal(int32(3)))
		Expect(final.Status.Summary.Passed).To(Equal(int32(3)))
	})

	It("warns about pod entries that do not match the Job's pod naming", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, recorder := newCheckRunReconciler(nil)

		run := newRun(ns, nil)
		reconcileRun(r, run)

		reportAsPod(run, defaultRunnerName, "forged-entry", []verikubedevv1alpha1.CheckResult{passResult("example-tcp")})
		markJobSucceeded(ns, jobName(run.Name, defaultRunnerName), 1)
		reconcileRun(r, run)

		found := false
		for len(recorder.Events) > 0 {
			if e := <-recorder.Events; strings.Contains(e, "ForeignResultEntry") {
				found = true
			}
		}
		Expect(found).To(BeTrue(), "expected a ForeignResultEntry warning event")
	})

	It("ignores terminal runs", func() {
		ns := createNamespace()
		createRunnerServiceAccount(ns)
		r, _ := newCheckRunReconciler(nil)

		run := newRun(ns, nil)
		reconcileRun(r, run)
		markJobFailed(ns, jobName(run.Name, defaultRunnerName), "boom")
		reconcileRun(r, run)
		Expect(getRun(run).Status.Phase).To(Equal(verikubedevv1alpha1.CheckRunError))

		// Another reconcile of a terminal run must be a no-op.
		res := reconcileRun(r, run)
		Expect(res).To(Equal(ctrl.Result{}))
	})
})

var _ = Describe("jobName", func() {
	It("keeps long names within the Job name budget", func() {
		long := strings.Repeat("abcdefgh", 30)
		name := jobName(long, "runner-with-a-long-name")
		Expect(len(name)).To(BeNumerically("<=", 57))
		// Deterministic: the same input yields the same name.
		Expect(jobName(long, "runner-with-a-long-name")).To(Equal(name))
		// Different inputs yield different names despite truncation.
		Expect(jobName(long+"x", "runner-with-a-long-name")).NotTo(Equal(name))
	})
})

var _ = Describe("aggregate", func() {
	It("reports incomplete while any runner has fewer entries than replicas", func() {
		run := &verikubedevv1alpha1.CheckRun{
			Spec: verikubedevv1alpha1.CheckRunSpec{
				Suite: verikubedevv1alpha1.CheckSuiteTemplate{
					Runners: []verikubedevv1alpha1.RunnerSpec{{Name: "a", Replicas: ptr.To(int32(2))}},
					Checks:  []verikubedevv1alpha1.CheckSpec{{Name: "c", TCP: &verikubedevv1alpha1.TCPCheck{Address: "x:1"}}},
				},
			},
			Status: verikubedevv1alpha1.CheckRunStatus{
				Runners: []verikubedevv1alpha1.RunnerStatus{
					{Name: "a", Pods: []verikubedevv1alpha1.PodResult{
						{PodName: "p1", Checks: []verikubedevv1alpha1.CheckResult{passResult("c")}},
					}},
				},
			},
		}
		complete, _, _ := aggregate(run)
		Expect(complete).To(BeFalse())
	})

	It("treats a pod entry missing a check result as a failure", func() {
		run := &verikubedevv1alpha1.CheckRun{
			Spec: verikubedevv1alpha1.CheckRunSpec{
				Suite: verikubedevv1alpha1.CheckSuiteTemplate{
					Runners: []verikubedevv1alpha1.RunnerSpec{{Name: "a"}},
					Checks: []verikubedevv1alpha1.CheckSpec{
						{Name: "c1", TCP: &verikubedevv1alpha1.TCPCheck{Address: "x:1"}},
						{Name: "c2", TCP: &verikubedevv1alpha1.TCPCheck{Address: "x:2"}},
					},
				},
			},
			Status: verikubedevv1alpha1.CheckRunStatus{
				Runners: []verikubedevv1alpha1.RunnerStatus{
					{Name: "a", Pods: []verikubedevv1alpha1.PodResult{
						{PodName: "p1", Checks: []verikubedevv1alpha1.CheckResult{passResult("c1")}},
					}},
				},
			},
		}
		complete, summary, failing := aggregate(run)
		Expect(complete).To(BeTrue())
		Expect(summary.Total).To(Equal(int32(2)))
		Expect(summary.Passed).To(Equal(int32(1)))
		Expect(summary.Failed).To(Equal(int32(1)))
		Expect(failing).To(HaveLen(1))
		Expect(failing[0]).To(ContainSubstring("c2"))
	})
})

var _ = Describe("buildJob argument propagation", func() {
	It("adds --allow-local-targets when enabled", func() {
		r := &CheckRunReconciler{RunnerImage: runnerImage, AllowLocalTargets: true, Scheme: k8sClient.Scheme()}
		run := &verikubedevv1alpha1.CheckRun{
			ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
			Spec:       verikubedevv1alpha1.CheckRunSpec{Suite: validTemplate()},
		}
		job, err := r.buildJob(run, run.Spec.Suite.Runners[0], time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(job.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{runnerSubcommand, "--allow-local-targets"}))
		Expect(fmt.Sprint(job.Spec.Template.Spec.Containers[0].Env)).To(ContainSubstring("VERIKUBE_CHECKRUN_NAME"))
	})
})
