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
	"slices"
	"time"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

const (
	// defaultStartingDeadline is how late a missed tick may still fire when
	// spec.startingDeadline is unset. It keeps unsuspends and operator
	// restarts from firing stale catch-up runs.
	defaultStartingDeadline = 200 * time.Second

	// maxMissedScans caps the missed-tick scan to guard against clock skew
	// and very frequent schedules after long downtime.
	maxMissedScans = 80

	// ConditionScheduleValid reports whether spec.schedule parses.
	ConditionScheduleValid = "ScheduleValid"
)

// CheckSuiteReconciler reconciles a CheckSuite object
type CheckSuiteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	// Clock is injectable for tests; defaults to the real clock.
	Clock clock.PassiveClock
}

// +kubebuilder:rbac:groups=verikube.dev,resources=checksuites,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=verikube.dev,resources=checksuites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=verikube.dev,resources=checksuites/finalizers,verbs=update
// +kubebuilder:rbac:groups=verikube.dev,resources=checkruns,verbs=get;list;watch;create;update;patch;delete

// Reconcile schedules CheckRuns for a suite (CronJob-style), handles manual
// run-now triggers, tracks active runs, and garbage-collects finished runs
// beyond the history limits.
func (r *CheckSuiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var suite verikubev1alpha1.CheckSuite
	if err := r.Get(ctx, req.NamespacedName, &suite); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	now := r.now()

	// Collect this suite's runs (labeled at creation, ownership verified).
	var runs verikubev1alpha1.CheckRunList
	if err := r.List(ctx, &runs, client.InNamespace(suite.Namespace),
		client.MatchingLabels{labelSuite: truncateName(suite.Name, 63)}); err != nil {
		return ctrl.Result{}, err
	}
	var active, succeeded, failed []verikubev1alpha1.CheckRun
	for _, run := range runs.Items {
		if !metav1.IsControlledBy(&run, &suite) {
			continue
		}
		switch run.Status.Phase {
		case verikubev1alpha1.CheckRunSucceeded:
			succeeded = append(succeeded, run)
		case verikubev1alpha1.CheckRunFailed, verikubev1alpha1.CheckRunError:
			failed = append(failed, run)
		default:
			active = append(active, run)
		}
	}

	status := suite.Status.DeepCopy()
	status.ObservedGeneration = suite.Generation
	status.Active = activeRefs(active)

	// History GC, split so failed runs can be kept longer for debugging.
	successLimit, failedLimit := historyLimits(suite.Spec.HistoryLimit)
	if err := r.gcRuns(ctx, succeeded, successLimit); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.gcRuns(ctx, failed, failedLimit); err != nil {
		return ctrl.Result{}, err
	}

	// Manual trigger: fires even while suspended — an explicit human action.
	if trigger := suite.Annotations[verikubev1alpha1.RunNowAnnotation]; trigger != "" && trigger != status.LastManualTrigger {
		name := truncateName(fmt.Sprintf("%s-manual-%s", suite.Name, hash8(trigger)), 63)
		run, err := r.createRun(ctx, &suite, name)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&suite, nil, corev1.EventTypeNormal, "ManualRunCreated", "Reconcile",
			"created CheckRun %s from %s annotation", name, verikubev1alpha1.RunNowAnnotation)
		status.LastManualTrigger = trigger
		status.Active = append(status.Active, runRef(run))
	}

	if ptr.Deref(suite.Spec.Suspend, false) || suite.Spec.Schedule == nil {
		return ctrl.Result{}, r.updateStatus(ctx, &suite, status)
	}

	sched, err := cron.ParseStandard(*suite.Spec.Schedule)
	if err != nil {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:               ConditionScheduleValid,
			Status:             metav1.ConditionFalse,
			Reason:             "ParseError",
			Message:            fmt.Sprintf("invalid schedule %q: %v", *suite.Spec.Schedule, err),
			ObservedGeneration: suite.Generation,
		})
		r.Recorder.Eventf(&suite, nil, corev1.EventTypeWarning, "InvalidSchedule", "Reconcile",
			"cannot parse schedule %q: %v", *suite.Spec.Schedule, err)
		// A spec change retriggers reconciliation; nothing to requeue.
		return ctrl.Result{}, r.updateStatus(ctx, &suite, status)
	}
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               ConditionScheduleValid,
		Status:             metav1.ConditionTrue,
		Reason:             "Parsed",
		Message:            "schedule parsed successfully",
		ObservedGeneration: suite.Generation,
	})

	// Schedules are evaluated in UTC.
	earliest := suite.CreationTimestamp.Time
	if status.LastScheduleTime != nil {
		earliest = status.LastScheduleTime.Time
	}
	lastMissed, next, tooMany := mostRecentScheduleTime(sched, earliest.UTC(), now.UTC())
	if tooMany {
		r.Recorder.Eventf(&suite, nil, corev1.EventTypeWarning, "TooManyMissedRuns", "Reconcile",
			"too many missed scheduled ticks; check clock skew or schedule frequency")
	}

	if lastMissed != nil {
		startingDeadline := defaultStartingDeadline
		if suite.Spec.StartingDeadline != nil {
			startingDeadline = suite.Spec.StartingDeadline.Duration
		}
		scheduled := metav1.NewTime(*lastMissed)
		switch {
		case now.Sub(*lastMissed) > startingDeadline:
			// Too stale to fire (e.g. just unsuspended or the operator was
			// down). Record it as handled so the scan does not grow.
			log.V(1).Info("skipping stale scheduled tick", "suite", req.NamespacedName, "scheduled", lastMissed)
			status.LastScheduleTime = &scheduled
		case suite.Spec.ConcurrencyPolicy == verikubev1alpha1.ForbidConcurrent && len(active) > 0:
			r.Recorder.Eventf(&suite, nil, corev1.EventTypeNormal, "RunSkipped", "Reconcile",
				"skipping scheduled run at %s: %d run(s) still active (concurrencyPolicy: Forbid)",
				lastMissed.Format(time.RFC3339), len(active))
			status.LastScheduleTime = &scheduled
		default:
			if suite.Spec.ConcurrencyPolicy == verikubev1alpha1.ReplaceConcurrent {
				for i := range active {
					if err := r.Delete(ctx, &active[i]); client.IgnoreNotFound(err) != nil {
						return ctrl.Result{}, err
					}
				}
			}
			name := truncateName(fmt.Sprintf("%s-%d", suite.Name, lastMissed.Unix()), 63)
			run, err := r.createRun(ctx, &suite, name)
			if err != nil {
				return ctrl.Result{}, err
			}
			log.Info("created scheduled CheckRun", "suite", req.NamespacedName, "checkrun", name, "scheduled", lastMissed)
			status.LastScheduleTime = &scheduled
			status.Active = append(status.Active, runRef(run))
		}
	}

	if err := r.updateStatus(ctx, &suite, status); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: next.Sub(now)}, nil
}

// mostRecentScheduleTime returns the most recent tick in (earliest, now],
// the next tick after now, and whether the scan hit its cap.
func mostRecentScheduleTime(sched cron.Schedule, earliest, now time.Time) (*time.Time, time.Time, bool) {
	var lastMissed *time.Time
	count := 0
	t := sched.Next(earliest)
	for !t.After(now) {
		tick := t
		lastMissed = &tick
		count++
		if count > maxMissedScans {
			return lastMissed, sched.Next(now), true
		}
		t = sched.Next(t)
	}
	return lastMissed, t, false
}

// createRun snapshots the suite template into a new CheckRun. Deterministic
// names make creation idempotent across reconciles of the same tick.
func (r *CheckSuiteReconciler) createRun(ctx context.Context, suite *verikubev1alpha1.CheckSuite, name string) (*verikubev1alpha1.CheckRun, error) {
	run := &verikubev1alpha1.CheckRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: suite.Namespace,
			Labels:    map[string]string{labelSuite: truncateName(suite.Name, 63)},
		},
		Spec: verikubev1alpha1.CheckRunSpec{
			SuiteRef: &corev1.LocalObjectReference{Name: suite.Name},
			Suite:    *suite.Spec.CheckSuiteTemplate.DeepCopy(),
		},
	}
	if err := ctrl.SetControllerReference(suite, run, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, run); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		// Idempotent re-create of the same tick: reuse the existing run.
		if err := r.Get(ctx, client.ObjectKeyFromObject(run), run); err != nil {
			return nil, err
		}
	}
	return run, nil
}

func runRef(run *verikubev1alpha1.CheckRun) corev1.ObjectReference {
	return corev1.ObjectReference{
		APIVersion: verikubev1alpha1.SchemeGroupVersion.String(),
		Kind:       "CheckRun",
		Namespace:  run.Namespace,
		Name:       run.Name,
		UID:        run.UID,
	}
}

// gcRuns deletes the oldest terminal runs beyond limit.
func (r *CheckSuiteReconciler) gcRuns(ctx context.Context, runs []verikubev1alpha1.CheckRun, limit int) error {
	if len(runs) <= limit {
		return nil
	}
	slices.SortFunc(runs, func(a, b verikubev1alpha1.CheckRun) int {
		return runRecency(&a).Compare(runRecency(&b))
	})
	excess := runs[:len(runs)-limit]
	for i := range excess {
		if err := r.Delete(ctx, &excess[i]); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

// runRecency orders runs for GC: prefer the actual start time and fall back
// to the creation timestamp (whose 1s resolution can tie).
func runRecency(run *verikubev1alpha1.CheckRun) time.Time {
	if run.Status.StartTime != nil {
		return run.Status.StartTime.Time
	}
	return run.CreationTimestamp.Time
}

func historyLimits(hl *verikubev1alpha1.HistoryLimit) (successful, failed int) {
	successful, failed = 3, 5
	if hl == nil {
		return successful, failed
	}
	if hl.Successful != nil {
		successful = int(*hl.Successful)
	}
	if hl.Failed != nil {
		failed = int(*hl.Failed)
	}
	return successful, failed
}

func activeRefs(runs []verikubev1alpha1.CheckRun) []corev1.ObjectReference {
	refs := make([]corev1.ObjectReference, 0, len(runs))
	for i := range runs {
		refs = append(refs, runRef(&runs[i]))
	}
	return refs
}

func (r *CheckSuiteReconciler) updateStatus(ctx context.Context, suite *verikubev1alpha1.CheckSuite, status *verikubev1alpha1.CheckSuiteStatus) error {
	suite.Status = *status
	return r.Status().Update(ctx, suite)
}

func (r *CheckSuiteReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock.Now()
	}
	return time.Now()
}

// SetupWithManager sets up the controller with the Manager.
func (r *CheckSuiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&verikubev1alpha1.CheckSuite{}).
		Owns(&verikubev1alpha1.CheckRun{}).
		Named("checksuite").
		Complete(r)
}
