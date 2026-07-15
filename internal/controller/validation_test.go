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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	verikubedevv1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

// These specs exercise the CEL rules on the installed CRDs through the
// envtest apiserver, i.e. exactly what users hit at admission time.
var _ = Describe("CheckSpec CEL validation", func() {
	var namespace string

	BeforeEach(func() {
		namespace = createNamespace()
	})

	newRun := func(check verikubedevv1alpha1.CheckSpec) *verikubedevv1alpha1.CheckRun {
		return &verikubedevv1alpha1.CheckRun{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "validation-", Namespace: namespace},
			Spec: verikubedevv1alpha1.CheckRunSpec{
				Suite: verikubedevv1alpha1.CheckSuiteTemplate{
					Runners: []verikubedevv1alpha1.RunnerSpec{{Name: defaultRunnerName}},
					Checks:  []verikubedevv1alpha1.CheckSpec{check},
				},
			},
		}
	}

	It("accepts a grpc-only check", func() {
		run := newRun(verikubedevv1alpha1.CheckSpec{
			Name: "grpc-ok",
			GRPC: &verikubedevv1alpha1.GRPCCheck{Address: "svc.example:50051", Service: "example.v1.Example"},
		})
		Expect(k8sClient.Create(ctx, run)).To(Succeed())
	})

	It("rejects a check with both tcp and grpc set", func() {
		run := newRun(verikubedevv1alpha1.CheckSpec{
			Name: "two-probes",
			TCP:  &verikubedevv1alpha1.TCPCheck{Address: "svc.example:443"},
			GRPC: &verikubedevv1alpha1.GRPCCheck{Address: "svc.example:50051"},
		})
		err := k8sClient.Create(ctx, run)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of tcp, http or grpc"))
	})

	It("rejects a check with no probe set", func() {
		run := newRun(verikubedevv1alpha1.CheckSpec{Name: "no-probe"})
		err := k8sClient.Create(ctx, run)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of tcp, http or grpc"))
	})

	It("rejects a grpc address carrying a scheme", func() {
		run := newRun(verikubedevv1alpha1.CheckSpec{
			Name: "grpc-scheme",
			GRPC: &verikubedevv1alpha1.GRPCCheck{Address: "grpc://svc.example:50051"},
		})
		err := k8sClient.Create(ctx, run)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("address must be host:port without a scheme"))
	})
})
