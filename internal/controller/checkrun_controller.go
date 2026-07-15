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
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
	"github.com/frauniki/verikube/internal/metrics"
	"github.com/frauniki/verikube/internal/runner"
)

const (
	// DefaultRunnerServiceAccount is the ServiceAccount runner pods use.
	// It must exist in every namespace that hosts CheckSuites (the Helm
	// chart creates it for each entry in checkNamespaces).
	DefaultRunnerServiceAccount = "verikube-runner"

	// defaultRunTimeout bounds a run when spec.suite.timeout is not set.
	defaultRunTimeout = 10 * time.Minute

	checkRunFieldManager = "verikube-checkrun-controller"

	// runnerSubcommand is both the container name and the binary subcommand
	// of runner pods.
	runnerSubcommand = "runner"
)

// CheckRunReconciler reconciles a CheckRun object
type CheckRunReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder

	// RunnerImage is the image runner Jobs use; defaults to the manager's
	// own image via the --runner-image flag.
	RunnerImage string
	// RunnerServiceAccount is the ServiceAccount name for runner pods.
	RunnerServiceAccount string
	// AllowLocalTargets is propagated to runners as --allow-local-targets.
	AllowLocalTargets bool
	// Clock is injectable for tests; defaults to the real clock.
	Clock clock.PassiveClock
}

// +kubebuilder:rbac:groups=verikube.dev,resources=checkruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=verikube.dev,resources=checkruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=verikube.dev,resources=checkruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile drives a CheckRun to a terminal phase: it creates one Job per
// runner, waits for runner pods to report results into status.runners[]
// (they patch it themselves via server-side apply), aggregates verdicts,
// and sets the final phase.
func (r *CheckRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var run verikubev1alpha1.CheckRun
	if err := r.Get(ctx, req.NamespacedName, &run); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if isTerminalPhase(run.Status.Phase) {
		return ctrl.Result{}, nil
	}

	now := r.now()
	timeout := defaultRunTimeout
	if run.Spec.Suite.Timeout != nil {
		timeout = run.Spec.Suite.Timeout.Duration
	}

	// The runner ServiceAccount is a hard prerequisite: without it Job pods
	// never start and the run would sit silently until the deadline. Fail
	// fast with an actionable message instead.
	if run.Status.StartTime == nil {
		var sa corev1.ServiceAccount
		saKey := types.NamespacedName{Namespace: run.Namespace, Name: r.runnerServiceAccount()}
		if err := r.Get(ctx, saKey, &sa); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			msg := fmt.Sprintf(
				"ServiceAccount %q not found in namespace %q; add the namespace to checkNamespaces in the verikube Helm values",
				r.runnerServiceAccount(), run.Namespace)
			r.Recorder.Eventf(&run, nil, corev1.EventTypeWarning, "RunnerServiceAccountMissing", "Reconcile", "%s", msg)
			return r.finish(ctx, &run, verikubev1alpha1.CheckRunError, now, nil, metav1.Condition{
				Type:    verikubev1alpha1.ConditionRunnerServiceAccountMissing,
				Status:  metav1.ConditionTrue,
				Reason:  "ServiceAccountNotFound",
				Message: msg,
			})
		}
	}

	// Ensure one Job per runner.
	created := false
	jobs := map[string]*batchv1.Job{}
	for _, rn := range run.Spec.Suite.Runners {
		var job batchv1.Job
		key := types.NamespacedName{Namespace: run.Namespace, Name: jobName(run.Name, rn.Name)}
		err := r.Get(ctx, key, &job)
		switch {
		case err == nil:
			jobs[rn.Name] = &job
		case apierrors.IsNotFound(err):
			desired, buildErr := r.buildJob(&run, rn, timeout)
			if buildErr != nil {
				return ctrl.Result{}, buildErr
			}
			if err := r.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
				return ctrl.Result{}, err
			}
			created = true
			jobs[rn.Name] = desired
		default:
			return ctrl.Result{}, err
		}
	}

	startTime := run.Status.StartTime
	if startTime == nil {
		t := metav1.NewTime(now)
		startTime = &t
	}
	if created {
		log.Info("runner jobs ensured", "checkrun", req.NamespacedName, "jobs", len(jobs))
	}

	// Deadline safety net: also rescues runs whose pods never became
	// schedulable (e.g. DoNotSchedule spread on a too-small cluster) and
	// runs whose runner results never landed.
	if now.After(startTime.Add(timeout)) {
		r.Recorder.Eventf(&run, nil, corev1.EventTypeWarning, "DeadlineExceeded", "Reconcile",
			"run did not complete within %s", timeout)
		return r.finish(ctx, &run, verikubev1alpha1.CheckRunError, now, nil, metav1.Condition{
			Type:    verikubev1alpha1.ConditionDeadlineExceeded,
			Status:  metav1.ConditionTrue,
			Reason:  "Timeout",
			Message: fmt.Sprintf("run did not complete within %s", timeout),
		})
	}

	// Any failed Job means the run could not execute (runner pods exit 0 on
	// check failures, so a Job failure is an infrastructure error).
	for rnName, job := range jobs {
		if jobFailed(job) {
			msg := fmt.Sprintf("runner Job %s failed: %s", job.Name, jobFailureMessage(job))
			r.Recorder.Eventf(&run, nil, corev1.EventTypeWarning, "RunnerError", "Reconcile", "%s", msg)
			return r.finish(ctx, &run, verikubev1alpha1.CheckRunError, now, nil, metav1.Condition{
				Type:    verikubev1alpha1.ConditionCompleted,
				Status:  metav1.ConditionTrue,
				Reason:  "RunnerError",
				Message: fmt.Sprintf("runner %s could not execute checks", rnName),
			})
		}
	}

	allSucceeded := true
	for _, job := range jobs {
		if !jobSucceeded(job) {
			allSucceeded = false
			break
		}
	}

	if allSucceeded {
		r.warnOnForeignPodEntries(&run, jobs)
		complete, summary, failing := aggregate(&run)
		if complete {
			phase := verikubev1alpha1.CheckRunSucceeded
			if summary.Failed > 0 {
				phase = verikubev1alpha1.CheckRunFailed
				for _, f := range failing {
					r.Recorder.Eventf(&run, nil, corev1.EventTypeWarning, "CheckFailed", "Reconcile", "%s", f)
				}
			}
			return r.finish(ctx, &run, phase, now, &summary, metav1.Condition{})
		}
		// Jobs are done but some pod entries have not landed yet. The SSA
		// patch and the pod's exit are separate API round-trips with no
		// ordering guarantee; the patch itself retriggers this reconciler,
		// so just wait — never declare Error on a short timer. The run
		// deadline above remains the only path to Error.
		log.V(1).Info("jobs complete, waiting for runner results to land", "checkrun", req.NamespacedName)
	}

	// Still running: record startTime/phase and requeue at the deadline.
	if err := r.applyStatus(ctx, &run, controllerStatus{
		phase:     verikubev1alpha1.CheckRunRunning,
		startTime: startTime,
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: startTime.Time.Add(timeout).Sub(now) + time.Second}, nil
}

// finish moves the run to a terminal phase, emits metrics, and applies the
// controller-owned status fields.
func (r *CheckRunReconciler) finish(ctx context.Context, run *verikubev1alpha1.CheckRun,
	phase verikubev1alpha1.CheckRunPhase, now time.Time, summary *verikubev1alpha1.RunSummary,
	extraCondition metav1.Condition,
) (ctrl.Result, error) {
	startTime := run.Status.StartTime
	if startTime == nil {
		t := metav1.NewTime(now)
		startTime = &t
	}
	completionTime := metav1.NewTime(now)

	st := controllerStatus{
		phase:          phase,
		startTime:      startTime,
		completionTime: &completionTime,
		summary:        summary,
	}
	if extraCondition.Type != "" {
		st.conditions = append(st.conditions, extraCondition)
	}
	st.conditions = append(st.conditions, metav1.Condition{
		Type:    verikubev1alpha1.ConditionCompleted,
		Status:  metav1.ConditionTrue,
		Reason:  string(phase),
		Message: fmt.Sprintf("run finished with phase %s", phase),
	})

	if err := r.applyStatus(ctx, run, st); err != nil {
		return ctrl.Result{}, err
	}
	r.recordMetrics(ctx, run, phase, completionTime.Sub(startTime.Time), completionTime.Time)
	if phase == verikubev1alpha1.CheckRunSucceeded {
		r.Recorder.Eventf(run, nil, corev1.EventTypeNormal, "Succeeded", "Reconcile", "all checks passed")
	}
	return ctrl.Result{}, nil
}

// controllerStatus is the set of status fields the controller owns. It is
// applied via SSA with the controller's field manager and must never
// include runners[], which runner pods own.
type controllerStatus struct {
	phase          verikubev1alpha1.CheckRunPhase
	startTime      *metav1.Time
	completionTime *metav1.Time
	summary        *verikubev1alpha1.RunSummary
	conditions     []metav1.Condition
}

// applyStatus server-side-applies only the controller-owned status fields.
// Using Status().Update here instead would write the whole status object
// from a possibly stale cache read and could roll back runner entries.
func (r *CheckRunReconciler) applyStatus(ctx context.Context, run *verikubev1alpha1.CheckRun, st controllerStatus) error {
	conditions := append([]metav1.Condition{}, run.Status.Conditions...)
	for _, c := range st.conditions {
		c.ObservedGeneration = run.Generation
		meta.SetStatusCondition(&conditions, c)
	}

	status := verikubev1alpha1.CheckRunStatus{
		ObservedGeneration: run.Generation,
		Phase:              st.phase,
		StartTime:          st.startTime,
		CompletionTime:     st.completionTime,
		Summary:            st.summary,
		Conditions:         conditions,
	}
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return err
	}
	// Defense in depth: the field is nil and omitted, but the controller
	// must never apply anything under runners[].
	delete(statusMap, "runners")

	doc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": verikubev1alpha1.SchemeGroupVersion.String(),
		"kind":       "CheckRun",
		"metadata": map[string]any{
			"name":      run.Name,
			"namespace": run.Namespace,
		},
		"status": statusMap,
	}}
	return r.Status().Apply(ctx, client.ApplyConfigurationFromUnstructured(doc), client.FieldOwner(checkRunFieldManager))
}

// aggregate decides whether every runner has a complete result set and
// computes the definition-based summary.
//
// Completeness is len(pods) >= replicas, not ==: a pod that patched its
// results and then died before exiting cleanly gets retried by the Job,
// leaving an extra (still valid) entry. Every entry is complete by
// construction because the patch is the runner's single last action.
func aggregate(run *verikubev1alpha1.CheckRun) (bool, verikubev1alpha1.RunSummary, []string) {
	podsByRunner := map[string][]verikubev1alpha1.PodResult{}
	for _, rs := range run.Status.Runners {
		podsByRunner[rs.Name] = rs.Pods
	}

	var summary verikubev1alpha1.RunSummary
	var failing []string
	for _, rn := range run.Spec.Suite.Runners {
		replicas := int(ptr.Deref(rn.Replicas, 1))
		pods := podsByRunner[rn.Name]
		if len(pods) < replicas {
			return false, verikubev1alpha1.RunSummary{}, nil
		}

		for _, chk := range runner.FilterChecks(run.Spec.Suite.Checks, rn.Name) {
			summary.Total++
			passed := true
			for _, pod := range pods {
				found := false
				for _, res := range pod.Checks {
					if res.Name == chk.Name {
						found = true
						if !res.Passed {
							passed = false
						}
						break
					}
				}
				if !found {
					passed = false
				}
			}
			if passed {
				summary.Passed++
			} else {
				summary.Failed++
				failing = append(failing, fmt.Sprintf("check %q failed on runner %q", chk.Name, rn.Name))
			}
		}
	}
	return true, summary, failing
}

// warnOnForeignPodEntries flags result entries whose pod name does not
// match the Job's pod naming convention — a cheap tamper indicator, since
// anything with pod-create rights in the namespace can borrow the runner
// ServiceAccount and forge entries.
func (r *CheckRunReconciler) warnOnForeignPodEntries(run *verikubev1alpha1.CheckRun, jobs map[string]*batchv1.Job) {
	for _, rs := range run.Status.Runners {
		job, ok := jobs[rs.Name]
		if !ok {
			r.Recorder.Eventf(run, nil, corev1.EventTypeWarning, "ForeignResultEntry", "Reconcile",
				"results reported for unknown runner %q", rs.Name)
			continue
		}
		for _, pod := range rs.Pods {
			if !strings.HasPrefix(pod.PodName, job.Name+"-") {
				r.Recorder.Eventf(run, nil, corev1.EventTypeWarning, "ForeignResultEntry", "Reconcile",
					"pod entry %q does not match runner Job %q pod naming", pod.PodName, job.Name)
			}
		}
	}
}

// buildJob renders the runner Job for one runner of the run.
func (r *CheckRunReconciler) buildJob(run *verikubev1alpha1.CheckRun, rn verikubev1alpha1.RunnerSpec, timeout time.Duration) (*batchv1.Job, error) {
	name := jobName(run.Name, rn.Name)
	replicas := ptr.Deref(rn.Replicas, 1)

	labels := map[string]string{
		labelCheckRun: truncateName(run.Name, 63),
		labelRunner:   rn.Name,
	}
	if run.Spec.SuiteRef != nil {
		labels[labelSuite] = truncateName(run.Spec.SuiteRef.Name, 63)
	}

	args := []string{runnerSubcommand}
	if r.AllowLocalTargets {
		args = append(args, "--allow-local-targets")
	}

	podSpec := corev1.PodSpec{
		RestartPolicy:      corev1.RestartPolicyNever,
		ServiceAccountName: r.runnerServiceAccount(),
		NodeSelector:       rn.NodeSelector,
		Tolerations:        rn.Tolerations,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{{
			Name:  runnerSubcommand,
			Image: r.RunnerImage,
			Args:  args,
			Env: []corev1.EnvVar{
				{Name: runner.EnvCheckRunName, Value: run.Name},
				{Name: runner.EnvCheckRunNamespace, Value: run.Namespace},
				{Name: runner.EnvRunnerName, Value: rn.Name},
				{
					Name:      runner.EnvPodName,
					ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}},
				},
				{
					Name:      runner.EnvNodeName,
					ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
				},
			},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			},
		}},
	}

	if rn.TopologySpread != nil {
		topologyKey := rn.TopologySpread.TopologyKey
		if topologyKey == "" {
			topologyKey = "topology.kubernetes.io/zone"
		}
		whenUnsatisfiable := rn.TopologySpread.WhenUnsatisfiable
		if whenUnsatisfiable == "" {
			whenUnsatisfiable = corev1.ScheduleAnyway
		}
		podSpec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
			MaxSkew:           1,
			TopologyKey:       topologyKey,
			WhenUnsatisfiable: whenUnsatisfiable,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{batchv1.JobNameLabel: name},
			},
		}}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: run.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Parallelism: ptr.To(replicas),
			Completions: ptr.To(replicas),
			// Check failures exit 0, so retries only fire on genuine
			// runner errors.
			BackoffLimit:          ptr.To(int32(2)),
			ActiveDeadlineSeconds: ptr.To(int64(timeout.Seconds())),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
	if err := ctrl.SetControllerReference(run, job, r.Scheme); err != nil {
		return nil, err
	}
	return job, nil
}

func (r *CheckRunReconciler) recordMetrics(ctx context.Context, run *verikubev1alpha1.CheckRun, phase verikubev1alpha1.CheckRunPhase, duration time.Duration, completedAt time.Time) {
	suite := ""
	if run.Spec.SuiteRef != nil {
		suite = run.Spec.SuiteRef.Name
	}
	ns := run.Namespace
	metrics.CheckRunsTotal.WithLabelValues(ns, suite, string(phase)).Inc()
	metrics.CheckRunDuration.WithLabelValues(ns, suite).Observe(duration.Seconds())

	// Error runs carry no check results; the gauges keep the last completed
	// run's values so staleness alerting can notice the gap.
	if phase != verikubev1alpha1.CheckRunSucceeded && phase != verikubev1alpha1.CheckRunFailed {
		return
	}
	// A check passes only if it passed on every pod that ran it. With
	// concurrencyPolicy Allow, runs can complete out of order and the last
	// one to finish wins the gauges.
	lastResults := map[string]bool{}
	for _, rs := range run.Status.Runners {
		for _, pod := range rs.Pods {
			for _, res := range pod.Checks {
				result := metrics.ResultPass
				if !res.Passed {
					result = metrics.ResultFail
				}
				metrics.CheckResultTotal.WithLabelValues(ns, suite, res.Name, result).Inc()
				if res.Duration != nil {
					metrics.CheckDuration.WithLabelValues(ns, suite, res.Name, result).Observe(res.Duration.Seconds())
				}
				passed, seen := lastResults[res.Name]
				lastResults[res.Name] = res.Passed && (!seen || passed)
			}
		}
	}
	metrics.SetLastResults(ns, suite, lastResults)
	metrics.CheckRunLastCompletion.WithLabelValues(ns, suite).Set(float64(completedAt.Unix()))

	// A run can outlive its suite, and the suite controller drops these
	// gauges exactly once, on the deletion event. Checking existence after
	// writing closes the race with that cleanup: both controllers read the
	// same informer cache, so if the suite is still visible here, its
	// deletion event — and the cleanup it triggers — is ordered after the
	// writes above; if it is already gone, drop the gauges ourselves.
	if run.Spec.SuiteRef == nil {
		return
	}
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: suite},
		&verikubev1alpha1.CheckSuite{}); apierrors.IsNotFound(err) {
		metrics.DeleteSuite(ns, suite)
	}
}

func (r *CheckRunReconciler) runnerServiceAccount() string {
	if r.RunnerServiceAccount != "" {
		return r.RunnerServiceAccount
	}
	return DefaultRunnerServiceAccount
}

func (r *CheckRunReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock.Now()
	}
	return time.Now()
}

func isTerminalPhase(phase verikubev1alpha1.CheckRunPhase) bool {
	switch phase {
	case verikubev1alpha1.CheckRunSucceeded, verikubev1alpha1.CheckRunFailed, verikubev1alpha1.CheckRunError:
		return true
	}
	return false
}

func jobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func jobFailureMessage(job *batchv1.Job) string {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return c.Message
		}
	}
	return ""
}

func jobSucceeded(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *CheckRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&verikubev1alpha1.CheckRun{}).
		Owns(&batchv1.Job{}).
		Named("checkrun").
		Complete(r)
}
