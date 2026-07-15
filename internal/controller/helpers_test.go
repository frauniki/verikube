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
	. "github.com/onsi/gomega"
	ioprometheusclient "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

// validTemplate returns a minimal CheckSuiteTemplate that passes CRD validation.
func validTemplate() verikubev1alpha1.CheckSuiteTemplate {
	return verikubev1alpha1.CheckSuiteTemplate{
		Runners: []verikubev1alpha1.RunnerSpec{
			{Name: defaultRunnerName},
		},
		Checks: []verikubev1alpha1.CheckSpec{
			{
				Name: "example-tcp",
				TCP:  &verikubev1alpha1.TCPCheck{Address: "example.com:443"},
			},
		},
	}
}

// suiteMetricLabels builds the label matcher shared by all verikube metrics.
func suiteMetricLabels(namespace, suite string) map[string]string {
	return map[string]string{"namespace": namespace, "suite": suite}
}

func checkMetricLabels(namespace, suite, check string) map[string]string {
	labels := suiteMetricLabels(namespace, suite)
	labels["check"] = check
	return labels
}

func checkResultMetricLabels(namespace, suite, check, result string) map[string]string {
	labels := checkMetricLabels(namespace, suite, check)
	labels["result"] = result
	return labels
}

// gaugeValue reads a gauge series from the controller-runtime registry
// without creating it, so absence is observable.
func gaugeValue(name string, labels map[string]string) (float64, bool) {
	m, ok := findMetric(name, labels)
	if !ok {
		return 0, false
	}
	return m.GetGauge().GetValue(), true
}

// histogramSampleCount reads a histogram series' observation count from the
// controller-runtime registry; 0 if the series does not exist.
func histogramSampleCount(name string, labels map[string]string) uint64 {
	m, ok := findMetric(name, labels)
	if !ok {
		return 0
	}
	return m.GetHistogram().GetSampleCount()
}

func findMetric(name string, labels map[string]string) (*ioprometheusclient.Metric, bool) {
	families, err := ctrlmetrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			have := map[string]string{}
			for _, lp := range m.GetLabel() {
				have[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range labels {
				if have[k] != v {
					match = false
					break
				}
			}
			if match {
				return m, true
			}
		}
	}
	return nil, false
}
