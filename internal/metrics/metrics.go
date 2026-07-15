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

const suiteLabel = "suite"

var (
	// CheckRunsTotal counts terminal CheckRun phases per suite.
	CheckRunsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "verikube_checkruns_total",
		Help: "Number of CheckRuns that reached a terminal phase.",
	}, []string{suiteLabel, "phase"})

	// CheckRunDuration observes wall-clock run durations per suite.
	CheckRunDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "verikube_checkrun_duration_seconds",
		Help:    "Duration of completed CheckRuns in seconds.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s .. ~68m
	}, []string{suiteLabel})

	// CheckResultTotal counts per-check verdicts for completed runs.
	CheckResultTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "verikube_check_result_total",
		Help: "Number of check verdicts observed in completed CheckRuns.",
	}, []string{suiteLabel, "check", "result"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(CheckRunsTotal, CheckRunDuration, CheckResultTotal)
}
