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

// Package metrics exposes verikube's Prometheus metrics on the manager's
// metrics endpoint. Labels deliberately exclude runner and pod names to
// keep cardinality bounded by what users define, not by execution volume.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	namespaceLabel = "namespace"
	suiteLabel     = "suite"
	checkLabel     = "check"
	resultLabel    = "result"
)

// Values for the result label.
const (
	ResultPass = "pass"
	ResultFail = "fail"
)

var (
	// CheckRunsTotal counts terminal CheckRun phases per suite.
	CheckRunsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "verikube_checkruns_total",
		Help: "Number of CheckRuns that reached a terminal phase.",
	}, []string{namespaceLabel, suiteLabel, "phase"})

	// CheckRunDuration observes wall-clock run durations per suite.
	CheckRunDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "verikube_checkrun_duration_seconds",
		Help:    "Duration of completed CheckRuns in seconds.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s .. ~68m
	}, []string{namespaceLabel, suiteLabel})

	// CheckResultTotal counts per-check verdicts for completed runs.
	CheckResultTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "verikube_check_result_total",
		Help: "Number of check verdicts observed in completed CheckRuns.",
	}, []string{namespaceLabel, suiteLabel, checkLabel, resultLabel})

	// CheckDuration observes individual probe latencies from completed runs.
	CheckDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "verikube_check_duration_seconds",
		Help:    "Duration of individual check probes in completed CheckRuns.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16), // 1ms .. ~32s
	}, []string{namespaceLabel, suiteLabel, checkLabel, resultLabel})

	// CheckLastResult reports the verdict of each check in the most recently
	// completed run: 1 = passed on every pod, 0 = failed on at least one.
	CheckLastResult = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "verikube_check_last_result",
		Help: "Latest verdict per check: 1 if it passed on every pod in the last completed run, 0 otherwise.",
	}, []string{namespaceLabel, suiteLabel, checkLabel})

	// CheckRunLastCompletion records when a suite last completed a run with
	// check results (Succeeded or Failed), for staleness alerting.
	CheckRunLastCompletion = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "verikube_checkrun_last_completion_timestamp_seconds",
		Help: "Unix timestamp of the suite's last completed CheckRun with results.",
	}, []string{namespaceLabel, suiteLabel})
)

// SetLastResults replaces the suite's per-check verdict gauges with the
// results of its latest completed run. Clearing first drops series for
// checks that were renamed or removed from the suite.
func SetLastResults(namespace, suite string, results map[string]bool) {
	CheckLastResult.DeletePartialMatch(prometheus.Labels{
		namespaceLabel: namespace,
		suiteLabel:     suite,
	})
	for check, passed := range results {
		value := 0.0
		if passed {
			value = 1.0
		}
		CheckLastResult.WithLabelValues(namespace, suite, check).Set(value)
	}
}

// DeleteSuite drops the suite's gauge series after the suite is deleted.
// Counters and histograms are left alone: stale but constant series are
// harmless to rate() queries, while stale gauges read as live state.
func DeleteSuite(namespace, suite string) {
	labels := prometheus.Labels{namespaceLabel: namespace, suiteLabel: suite}
	CheckLastResult.DeletePartialMatch(labels)
	CheckRunLastCompletion.DeletePartialMatch(labels)
}

func init() {
	ctrlmetrics.Registry.MustRegister(
		CheckRunsTotal,
		CheckRunDuration,
		CheckResultTotal,
		CheckDuration,
		CheckLastResult,
		CheckRunLastCompletion,
	)
}
