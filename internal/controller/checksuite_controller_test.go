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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robfig/cron/v3"
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
	"github.com/frauniki/verikube/internal/metrics"
)

func newCheckSuiteReconciler(clk *testclock.FakePassiveClock) (*CheckSuiteReconciler, *events.FakeRecorder) {
	recorder := events.NewFakeRecorder(100)
	r := &CheckSuiteReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: recorder,
	}
	if clk != nil {
		r.Clock = clk
	}
	return r, recorder
}

func reconcileSuite(r *CheckSuiteReconciler, suite *verikubedevv1alpha1.CheckSuite) ctrl.Result {
	res, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: suite.Name, Namespace: suite.Namespace},
	})
	Expect(err).NotTo(HaveOccurred())
	return res
}

func getSuite(suite *verikubedevv1alpha1.CheckSuite) *verikubedevv1alpha1.CheckSuite {
	out := &verikubedevv1alpha1.CheckSuite{}
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(suite), out)).To(Succeed())
	return out
}

func listSuiteRuns(suite *verikubedevv1alpha1.CheckSuite) []verikubedevv1alpha1.CheckRun {
	runs := &verikubedevv1alpha1.CheckRunList{}
	Expect(k8sClient.List(ctx, runs, client.InNamespace(suite.Namespace),
		client.MatchingLabels{labelSuite: truncateName(suite.Name, 63)})).To(Succeed())
	return runs.Items
}

var _ = Describe("CheckSuite Controller", func() {
	newSuite := func(namespace string, mutate func(*verikubedevv1alpha1.CheckSuite)) *verikubedevv1alpha1.CheckSuite {
		suite := &verikubedevv1alpha1.CheckSuite{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "suite-", Namespace: namespace},
			Spec: verikubedevv1alpha1.CheckSuiteSpec{
				CheckSuiteTemplate: validTemplate(),
			},
		}
		if mutate != nil {
			mutate(suite)
		}
		Expect(k8sClient.Create(ctx, suite)).To(Succeed())
		return suite
	}

	scheduledEvery5m := func(suite *verikubedevv1alpha1.CheckSuite) {
		suite.Spec.Schedule = ptr.To("*/5 * * * *")
		// Generous deadline so a tick anywhere in the 5m window still fires.
		suite.Spec.StartingDeadline = &metav1.Duration{Duration: 10 * time.Minute}
	}

	It("creates a snapshot CheckRun when a scheduled tick is due, idempotently", func() {
		ns := createNamespace()
		clk := testclock.NewFakePassiveClock(time.Now())
		r, _ := newCheckSuiteReconciler(clk)

		suite := newSuite(ns, scheduledEvery5m)
		clk.SetTime(clk.Now().Add(6 * time.Minute)) // at least one tick elapsed
		res := reconcileSuite(r, suite)

		runs := listSuiteRuns(suite)
		Expect(runs).To(HaveLen(1))
		run := runs[0]
		Expect(run.Spec.SuiteRef.Name).To(Equal(suite.Name))
		Expect(run.Spec.Suite).To(Equal(suite.Spec.CheckSuiteTemplate), "run must snapshot the template")
		Expect(metav1.IsControlledBy(&run, suite)).To(BeTrue())

		updated := getSuite(suite)
		Expect(updated.Status.LastScheduleTime).NotTo(BeNil())
		Expect(updated.Status.Active).To(HaveLen(1), "freshly created run must be tracked as active")
		cond := meta.FindStatusCondition(updated.Status.Conditions, ConditionScheduleValid)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(res.RequeueAfter).To(BeNumerically(">", 0), "must requeue for the next tick")

		// Same clock, second reconcile: the tick was recorded, no new run.
		reconcileSuite(r, suite)
		Expect(listSuiteRuns(suite)).To(HaveLen(1))
	})

	It("skips the scheduled run while one is active under Forbid", func() {
		ns := createNamespace()
		clk := testclock.NewFakePassiveClock(time.Now())
		r, recorder := newCheckSuiteReconciler(clk)

		suite := newSuite(ns, scheduledEvery5m) // concurrencyPolicy defaults to Forbid
		clk.SetTime(clk.Now().Add(6 * time.Minute))
		reconcileSuite(r, suite)
		Expect(listSuiteRuns(suite)).To(HaveLen(1))

		// Next tick, previous run still active (no phase set).
		clk.SetTime(clk.Now().Add(5 * time.Minute))
		reconcileSuite(r, suite)

		Expect(listSuiteRuns(suite)).To(HaveLen(1), "Forbid must not create an overlapping run")
		found := false
		for len(recorder.Events) > 0 {
			if e := <-recorder.Events; strings.Contains(e, "RunSkipped") {
				found = true
			}
		}
		Expect(found).To(BeTrue(), "expected a RunSkipped event")
	})

	It("replaces the active run on the next tick under Replace", func() {
		ns := createNamespace()
		clk := testclock.NewFakePassiveClock(time.Now())
		r, _ := newCheckSuiteReconciler(clk)

		suite := newSuite(ns, func(s *verikubedevv1alpha1.CheckSuite) {
			scheduledEvery5m(s)
			s.Spec.ConcurrencyPolicy = verikubedevv1alpha1.ReplaceConcurrent
		})
		clk.SetTime(clk.Now().Add(6 * time.Minute))
		reconcileSuite(r, suite)
		first := listSuiteRuns(suite)
		Expect(first).To(HaveLen(1))

		clk.SetTime(clk.Now().Add(5 * time.Minute))
		reconcileSuite(r, suite)

		runs := listSuiteRuns(suite)
		Expect(runs).To(HaveLen(1), "Replace must delete the active run and create a new one")
		Expect(runs[0].Name).NotTo(Equal(first[0].Name))
	})

	It("skips stale ticks beyond startingDeadline instead of firing catch-up runs", func() {
		ns := createNamespace()
		now := time.Now()
		clk := testclock.NewFakePassiveClock(now)
		r, _ := newCheckSuiteReconciler(clk)

		// Hourly schedule whose tick lands ~5 minutes after creation; by the
		// time the operator "comes back" 30 minutes later, that tick is ~25
		// minutes stale — far beyond the default 200s startingDeadline.
		tickMinute := (now.UTC().Minute() + 5) % 60
		suite := newSuite(ns, func(s *verikubedevv1alpha1.CheckSuite) {
			s.Spec.Schedule = ptr.To(fmt.Sprintf("%d * * * *", tickMinute))
			// default startingDeadline (200s) applies
		})
		clk.SetTime(now.Add(30 * time.Minute))
		reconcileSuite(r, suite)

		Expect(listSuiteRuns(suite)).To(BeEmpty(), "stale ticks must not fire")
		updated := getSuite(suite)
		Expect(updated.Status.LastScheduleTime).NotTo(BeNil(),
			"skipped ticks must still be recorded as handled")
	})

	It("does not schedule while suspended, but still honors run-now", func() {
		ns := createNamespace()
		clk := testclock.NewFakePassiveClock(time.Now())
		r, _ := newCheckSuiteReconciler(clk)

		suite := newSuite(ns, func(s *verikubedevv1alpha1.CheckSuite) {
			scheduledEvery5m(s)
			s.Spec.Suspend = ptr.To(true)
		})
		clk.SetTime(clk.Now().Add(6 * time.Minute))
		reconcileSuite(r, suite)
		Expect(listSuiteRuns(suite)).To(BeEmpty(), "suspended suites must not schedule")

		// Manual trigger works even while suspended.
		latest := getSuite(suite)
		latest.Annotations = map[string]string{verikubedevv1alpha1.RunNowAnnotation: "trigger-1"}
		Expect(k8sClient.Update(ctx, latest)).To(Succeed())
		reconcileSuite(r, suite)

		runs := listSuiteRuns(suite)
		Expect(runs).To(HaveLen(1))
		Expect(runs[0].Name).To(ContainSubstring("-manual-"))
		Expect(getSuite(suite).Status.LastManualTrigger).To(Equal("trigger-1"))

		// Idempotent for the same trigger value.
		reconcileSuite(r, suite)
		Expect(listSuiteRuns(suite)).To(HaveLen(1))

		// A new value fires a new run.
		latest = getSuite(suite)
		latest.Annotations[verikubedevv1alpha1.RunNowAnnotation] = "trigger-2"
		Expect(k8sClient.Update(ctx, latest)).To(Succeed())
		reconcileSuite(r, suite)
		Expect(listSuiteRuns(suite)).To(HaveLen(2))
	})

	It("reports invalid cron schedules via the ScheduleValid condition", func() {
		ns := createNamespace()
		r, recorder := newCheckSuiteReconciler(nil)

		suite := newSuite(ns, func(s *verikubedevv1alpha1.CheckSuite) {
			s.Spec.Schedule = ptr.To("this is not cron")
		})
		res := reconcileSuite(r, suite)

		updated := getSuite(suite)
		cond := meta.FindStatusCondition(updated.Status.Conditions, ConditionScheduleValid)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(res.RequeueAfter).To(BeZero(), "invalid schedule must not requeue-loop")
		Eventually(recorder.Events).Should(Receive(ContainSubstring("InvalidSchedule")))
	})

	It("garbage-collects terminal runs beyond the history limits", func() {
		ns := createNamespace()
		r, _ := newCheckSuiteReconciler(nil)

		suite := newSuite(ns, func(s *verikubedevv1alpha1.CheckSuite) {
			s.Spec.HistoryLimit = &verikubedevv1alpha1.HistoryLimit{
				Successful: ptr.To(int32(1)),
				Failed:     ptr.To(int32(1)),
			}
		})

		base := time.Now().Add(-1 * time.Hour)
		makeTerminalRun := func(i int, phase verikubedevv1alpha1.CheckRunPhase) string {
			name := fmt.Sprintf("%s-hist-%s-%d", suite.Name, strings.ToLower(string(phase)), i)
			_, err := r.createRun(ctx, getSuite(suite), name)
			Expect(err).NotTo(HaveOccurred())
			run := &verikubedevv1alpha1.CheckRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, run)).To(Succeed())
			start := metav1.NewTime(base.Add(time.Duration(i) * time.Minute))
			run.Status.Phase = phase
			run.Status.StartTime = &start
			Expect(k8sClient.Status().Update(ctx, run)).To(Succeed())
			return name
		}

		newestSucceeded := ""
		for i := range 3 {
			newestSucceeded = makeTerminalRun(i, verikubedevv1alpha1.CheckRunSucceeded)
		}
		newestFailed := ""
		for i := range 3 {
			newestFailed = makeTerminalRun(i, verikubedevv1alpha1.CheckRunFailed)
		}

		reconcileSuite(r, suite)

		runs := listSuiteRuns(suite)
		Expect(runs).To(HaveLen(2), "one successful and one failed run must survive")
		names := map[string]bool{}
		for _, run := range runs {
			names[run.Name] = true
		}
		Expect(names).To(HaveKey(newestSucceeded), "the newest successful run must be kept")
		Expect(names).To(HaveKey(newestFailed), "the newest failed run must be kept")
	})

	It("does nothing schedule-wise when no schedule is set", func() {
		ns := createNamespace()
		r, _ := newCheckSuiteReconciler(nil)

		suite := newSuite(ns, nil)
		res := reconcileSuite(r, suite)
		Expect(res.RequeueAfter).To(BeZero())
		Expect(listSuiteRuns(suite)).To(BeEmpty())
	})

	It("drops state gauges when the suite is deleted", func() {
		ns := createNamespace()
		r, _ := newCheckSuiteReconciler(nil)
		suite := newSuite(ns, nil)
		reconcileSuite(r, suite)

		metrics.SetLastResults(ns, suite.Name, map[string]bool{"example-tcp": true})
		metrics.CheckRunLastCompletion.WithLabelValues(ns, suite.Name).Set(123)

		Expect(k8sClient.Delete(ctx, suite)).To(Succeed())
		reconcileSuite(r, suite) // NotFound path

		_, ok := gaugeValue("verikube_check_last_result", suiteMetricLabels(ns, suite.Name))
		Expect(ok).To(BeFalse())
		_, ok = gaugeValue("verikube_checkrun_last_completion_timestamp_seconds",
			suiteMetricLabels(ns, suite.Name))
		Expect(ok).To(BeFalse())
	})
})

var _ = Describe("mostRecentScheduleTime", func() {
	parse := func(expr string) cron.Schedule {
		sched, err := cron.ParseStandard(expr)
		Expect(err).NotTo(HaveOccurred())
		return sched
	}

	It("returns the most recent missed tick and the next one", func() {
		sched := parse("0 * * * *") // hourly at :00
		base := time.Date(2026, 7, 15, 9, 30, 0, 0, time.UTC)
		now := base.Add(2 * time.Hour) // 11:30

		last, next, tooMany := mostRecentScheduleTime(sched, base, now)
		Expect(tooMany).To(BeFalse())
		Expect(last).NotTo(BeNil())
		Expect(*last).To(Equal(time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)))
		Expect(next).To(Equal(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)))
	})

	It("returns nil when no tick has passed", func() {
		sched := parse("0 * * * *")
		base := time.Date(2026, 7, 15, 9, 30, 0, 0, time.UTC)

		last, next, tooMany := mostRecentScheduleTime(sched, base, base.Add(10*time.Minute))
		Expect(tooMany).To(BeFalse())
		Expect(last).To(BeNil())
		Expect(next).To(Equal(time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)))
	})

	It("caps the scan for very long gaps", func() {
		sched := parse("* * * * *") // every minute
		base := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

		last, _, tooMany := mostRecentScheduleTime(sched, base, base.Add(24*time.Hour))
		Expect(tooMany).To(BeTrue())
		Expect(last).NotTo(BeNil())
	})
})
