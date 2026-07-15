//go:build e2e
// +build e2e

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

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/frauniki/verikube/test/utils"
)

// kubectl runs kubectl with the given args and returns its output.
func kubectl(args ...string) (string, error) {
	return utils.Run(exec.Command("kubectl", args...))
}

// runPhase returns the phase(s) of the CheckRuns of the given suite.
func runPhase(suite string) string {
	out, err := kubectl("get", "checkrun", "-n", testNamespace,
		"-l", "verikube.dev/suite="+suite,
		"-o", "jsonpath={.items[*].status.phase}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// triggerRunNow fires a manual run of the suite via the annotation.
func triggerRunNow(suite string) {
	_, err := kubectl("annotate", "checksuite", suite, "-n", testNamespace,
		fmt.Sprintf("verikube.dev/run-now=%d", time.Now().UnixNano()), "--overwrite")
	Expect(err).NotTo(HaveOccurred())
}

// applyStdin applies a manifest passed on stdin.
func applyStdin(manifest string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("verikube operator", Ordered, func() {
	BeforeAll(func() {
		By("creating the test namespace")
		_, _ = kubectl("create", "namespace", testNamespace)

		By("deploying nginx as a check target")
		_, err := kubectl("apply", "-f", "test/e2e/testdata/nginx.yaml")
		Expect(err).NotTo(HaveOccurred())

		By("deploying the gRPC health server as a check target")
		_, err = kubectl("apply", "-f", "test/e2e/testdata/grpc-server.yaml")
		Expect(err).NotTo(HaveOccurred())

		_, err = kubectl("wait", "--for=condition=Ready", "pod/nginx", "pod/grpc-server",
			"-n", testNamespace, "--timeout=120s")
		Expect(err).NotTo(HaveOccurred(), "check target pods did not become ready")
	})

	AfterAll(func() {
		By("cleaning up the test namespace")
		_, _ = kubectl("delete", "namespace", testNamespace, "--ignore-not-found")
	})

	It("runs a passing suite to Succeeded, including a negative check", func() {
		_, err := kubectl("apply", "-f", "test/e2e/testdata/suite-pass.yaml")
		Expect(err).NotTo(HaveOccurred())

		triggerRunNow("e2e-pass")

		Eventually(func() string {
			return runPhase("e2e-pass")
		}, 3*time.Minute, 5*time.Second).Should(Equal("Succeeded"))

		By("verifying the negative check observed a failure but passed")
		out, err := kubectl("get", "checkrun", "-n", testNamespace,
			"-l", "verikube.dev/suite=e2e-pass",
			"-o", "jsonpath={.items[0].status.runners[0].pods[0].checks[?(@.name=='closed-port-blocked')]}")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"observed":"Failure"`))
		Expect(out).To(ContainSubstring(`"passed":true`))

		By("verifying the summary counts all checks as passed")
		out, err = kubectl("get", "checkrun", "-n", testNamespace,
			"-l", "verikube.dev/suite=e2e-pass",
			"-o", "jsonpath={.items[0].status.summary}")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(`"total":4`))
		Expect(out).To(ContainSubstring(`"passed":4`))
	})

	It("runs a suite with a wrong expectation to Failed", func() {
		_, err := kubectl("apply", "-f", "test/e2e/testdata/suite-fail.yaml")
		Expect(err).NotTo(HaveOccurred())

		triggerRunNow("e2e-fail")

		Eventually(func() string {
			return runPhase("e2e-fail")
		}, 3*time.Minute, 5*time.Second).Should(Equal("Failed"))

		By("verifying the failure message names the unexpected status")
		out, err := kubectl("get", "checkrun", "-n", testNamespace,
			"-l", "verikube.dev/suite=e2e-fail",
			"-o", "jsonpath={.items[0].status.runners[0].pods[0].checks[0].message}")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("unexpected status code: got 200"))
	})

	It("re-triggers a suite with a new run-now value", func() {
		triggerRunNow("e2e-pass")

		Eventually(func() int {
			out, err := kubectl("get", "checkrun", "-n", testNamespace,
				"-l", "verikube.dev/suite=e2e-pass",
				"-o", "jsonpath={.items[*].metadata.name}")
			if err != nil {
				return 0
			}
			return len(strings.Fields(out))
		}, 2*time.Minute, 5*time.Second).Should(BeNumerically(">=", 2),
			"a changed run-now value must create a second run")
	})

	It("fails fast in a namespace without the runner ServiceAccount", func() {
		const ns = "verikube-e2e-nosa"
		_, _ = kubectl("create", "namespace", ns)
		defer func() { _, _ = kubectl("delete", "namespace", ns, "--ignore-not-found") }()

		suite := strings.ReplaceAll(`apiVersion: verikube.dev/v1alpha1
kind: CheckSuite
metadata:
  name: e2e-nosa
  namespace: NS
spec:
  runners: [{name: default}]
  checks:
    - name: never-runs
      tcp: {address: "example.com:443"}
`, "NS", ns)
		applyStdin(suite)

		_, err := kubectl("annotate", "checksuite", "e2e-nosa", "-n", ns,
			fmt.Sprintf("verikube.dev/run-now=%d", time.Now().UnixNano()), "--overwrite")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() string {
			out, err := kubectl("get", "checkrun", "-n", ns,
				"-l", "verikube.dev/suite=e2e-nosa",
				"-o", "jsonpath={.items[*].status.phase}")
			if err != nil {
				return ""
			}
			return strings.TrimSpace(out)
		}, 2*time.Minute, 5*time.Second).Should(Equal("Error"),
			"missing runner ServiceAccount must fail fast with phase Error")
	})
})
