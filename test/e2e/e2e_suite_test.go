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
	"os"
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/frauniki/verikube/test/utils"
)

const (
	// managerImage is built and side-loaded into the kind cluster.
	managerImage = "example.com/verikube:v0.0.1"

	operatorNamespace = "verikube-system"
	testNamespace     = "verikube-e2e"
	releaseName       = "verikube"

	// nginxBaseImage is the upstream image the check target is derived from.
	nginxBaseImage = "nginx:alpine"
	// nginxImage is a locally built alias of nginxBaseImage, preloaded into
	// kind so the e2e run has no registry dependency (Docker Hub rate
	// limits are a classic CI flake). The one-line local build also keeps
	// `kind load` working on Docker installations using the containerd
	// image store, where side-loading registry-pulled multi-arch images
	// fails with "content digest ... not found".
	nginxImage = "verikube-e2e/nginx:local"

	// agnhostBaseImage provides the gRPC health-check server target
	// (`agnhost grpc-health-checking` serves grpc.health.v1.Health on :5000).
	agnhostBaseImage = "registry.k8s.io/e2e-test-images/agnhost:2.53"
	// agnhostImage is its locally built alias, preloaded for the same
	// reasons as nginxImage.
	agnhostImage = "verikube-e2e/agnhost:local"
)

// buildAndLoadWrappedImage builds a single-line FROM wrapper of base as tag
// and side-loads it into the kind cluster.
func buildAndLoadWrappedImage(base, tag string) {
	buildCmd := exec.Command("docker", "build", "-t", tag, "-")
	buildCmd.Stdin = strings.NewReader("FROM " + base + "\n")
	_, err := utils.Run(buildCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build the wrapped image "+tag)
	Expect(utils.LoadImageToKindClusterWithName(tag)).To(Succeed(),
		"Failed to load "+tag+" into kind")
}

// TestE2E deploys the operator with the Helm chart onto a kind cluster and
// exercises the full check lifecycle against a real nginx workload.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting verikube e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	// E2E_SKIP_BUILD=true skips the image build and uses a prebuilt
	// example.com/verikube:v0.0.1 from the local docker daemon — useful for
	// reusing a CI-built image or for hosts where in-container builds are
	// unavailable. The image is still loaded into kind either way.
	var err error
	if os.Getenv("E2E_SKIP_BUILD") != "true" {
		By("building the operator image")
		cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to build the operator image")
	}

	By("loading the operator image into kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	Expect(err).NotTo(HaveOccurred(), "Failed to load the operator image into kind")

	By("building and preloading the nginx test image into kind")
	buildAndLoadWrappedImage(nginxBaseImage, nginxImage)

	By("building and preloading the agnhost gRPC test image into kind")
	buildAndLoadWrappedImage(agnhostBaseImage, agnhostImage)

	By("creating the check namespace (the chart provisions the runner ServiceAccount into it)")
	cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
	_, _ = utils.Run(cmd) // ignore AlreadyExists

	By("installing the operator with the Helm chart")
	cmd = exec.Command("helm", "upgrade", "--install", releaseName, "charts/verikube",
		"--namespace", operatorNamespace, "--create-namespace",
		"--set", "image.repository=example.com/verikube",
		"--set", "image.tag=v0.0.1",
		"--set", fmt.Sprintf("checkNamespaces={%s}", testNamespace),
		"--wait", "--timeout", "3m",
	)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install the Helm chart")
})

var _ = AfterSuite(func() {
	By("collecting operator logs for debugging")
	cmd := exec.Command("kubectl", "logs", "-n", operatorNamespace,
		"deployment/"+releaseName, "--tail", "100")
	if out, err := utils.Run(cmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "operator logs:\n%s\n", out)
	}

	By("uninstalling the Helm release")
	cmd = exec.Command("helm", "uninstall", releaseName, "--namespace", operatorNamespace)
	_, _ = utils.Run(cmd)
})
