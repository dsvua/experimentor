/*
Copyright 2025.

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

	"experimentcontroller.example.com/experiment-deployment/test/utils"
)

// namespace where the project is deployed in
const namespace = "experimentor-system"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, and deploying
	// the controller and test services using Helm.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing experiment-controller using Helm")
		cmd = exec.Command("helm", "install", "experiment-controller", "./charts/experiment-controller",
			"--namespace", namespace,
			"--set", "image.repository=example.com/experimentor",
			"--set", "image.tag=v0.0.1",
			"--set", "image.pullPolicy=Never")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install experiment-controller via Helm")

		By("installing ping-pong deployment test service using Helm")
		cmd = exec.Command("helm", "install", "ping-pong-test", "./charts/ping-pong",
			"--namespace", "default",
			"--set", "image.repository=nginxinc/nginx-unprivileged",
			"--set", "image.tag=1.21",
			"--set", "service.containerPort=8080")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install ping-pong test service via Helm")

		By("installing ping-pong StatefulSet test service using Helm")
		cmd = exec.Command("helm", "install", "ping-pong-sts-test", "./charts/ping-pong-statefulset",
			"--namespace", "default",
			"--set", "persistence.enabled=false",
			"--set", "image.repository=nginxinc/nginx-unprivileged",
			"--set", "image.tag=1.21",
			"--set", "service.containerPort=8080")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install ping-pong StatefulSet test service via Helm")
	})

	// After all tests have been executed, clean up by uninstalling Helm releases
	// and deleting the namespace.
	AfterAll(func() {

		By("uninstalling ping-pong StatefulSet test service")
		cmd := exec.Command("helm", "uninstall", "ping-pong-sts-test", "--namespace", "default")
		_, _ = utils.Run(cmd)

		By("uninstalling ping-pong test service")
		cmd = exec.Command("helm", "uninstall", "ping-pong-test", "--namespace", "default")
		_, _ = utils.Run(cmd)

		By("uninstalling experiment-controller")
		cmd = exec.Command("helm", "uninstall", "experiment-controller", "--namespace", namespace)
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "app.kubernetes.io/name=experiment-controller",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("experiment-controller"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should handle ExperimentDeployment lifecycle", func() {
			By("waiting for ping-pong deployment to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "ping-pong-test", "-n", "default",
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("3"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying ExperimentDeployment CRs are created by Helm")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "experimentdeployment", "ping-pong-test-experiment", "-n", "default")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}, 1*time.Minute).Should(Succeed())

			By("verifying experiment deployment is created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "ping-pong-test-experiment", "-n", "default",
					"-o", "jsonpath={.spec.replicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying experiment deployment has correct image override")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "ping-pong-test-experiment", "-n", "default",
					"-o", "jsonpath={.spec.template.spec.containers[0].image}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("nginx"))
			}, 1*time.Minute).Should(Succeed())

			By("verifying experiment deployment has experiment labels")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "ping-pong-test-experiment", "-n", "default",
					"-o", "jsonpath={.spec.template.metadata.labels['experiment-controller\\.example\\.com/role']}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("experiment"))
			}, 1*time.Minute).Should(Succeed())

			By("verifying ExperimentDeployment status is updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "experimentdeployment", "ping-pong-test-experiment", "-n", "default",
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}, 3*time.Minute).Should(Succeed())

			// No need to delete experiment as it's managed by Helm lifecycle
		})

		It("should handle missing source deployment", func() {
			By("creating an ExperimentDeployment CR with non-existent source")
			experimentCRManifest := `
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: experiment-missing-source
  namespace: experimentor-system
spec:
  sourceRef:
    kind: Deployment
    name: non-existent-deployment
    namespace: experimentor-system
  replicas: 1
  overrideSpec: {}
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(experimentCRManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ExperimentDeployment CR")

			By("verifying ExperimentDeployment status shows source not found")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "experimentdeployment", "experiment-missing-source", "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Synced')].reason}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("SourceNotFound"))
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "experimentdeployment", "experiment-missing-source", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should handle cross-namespace source references", func() {
			testNamespace := "test-source-namespace"

			By("creating test namespace")
			cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")

			defer func() {
				By("cleaning up test namespace")
				cmd := exec.Command("kubectl", "delete", "namespace", testNamespace)
				_, _ = utils.Run(cmd)
			}()

			By("creating source deployment in different namespace")
			sourceDeploymentManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cross-namespace-source
  namespace: %s
spec:
  replicas: 2
  selector:
    matchLabels:
      app: cross-namespace-app
  template:
    metadata:
      labels:
        app: cross-namespace-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:1.21
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - "ALL"
          runAsNonRoot: true
          runAsUser: 101
          seccompProfile:
            type: RuntimeDefault
`, testNamespace)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sourceDeploymentManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create cross-namespace source deployment")

			By("waiting for cross-namespace source deployment to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "cross-namespace-source", "-n", testNamespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("2"))
			}, 2*time.Minute).Should(Succeed())

			By("creating ExperimentDeployment with cross-namespace source")
			experimentCRManifest := fmt.Sprintf(`
apiVersion: experimentcontroller.example.com/v1alpha1
kind: ExperimentDeployment
metadata:
  name: experiment-cross-namespace
  namespace: %s
spec:
  sourceRef:
    kind: Deployment
    name: cross-namespace-source
    namespace: %s
  replicas: 1
  overrideSpec: {}
`, namespace, testNamespace)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(experimentCRManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create cross-namespace ExperimentDeployment CR")

			By("verifying experiment deployment is created in CR namespace")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "experiment-cross-namespace", "-n", namespace,
					"-o", "jsonpath={.spec.replicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "experimentdeployment", "experiment-cross-namespace", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should handle StatefulSet sources", func() {
			By("waiting for ping-pong StatefulSet to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "statefulset", "ping-pong-sts-test-ping-pong-statefulset", "-n", "default",
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("3"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying ExperimentDeployment CR for StatefulSet is created by Helm")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "experimentdeployment", "ping-pong-statefulset-experiment", "-n", "default")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}, 1*time.Minute).Should(Succeed())

			By("verifying experiment StatefulSet is created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "statefulset", "ping-pong-statefulset-experiment", "-n", "default",
					"-o", "jsonpath={.spec.replicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying experiment StatefulSet has correct image override")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "statefulset", "ping-pong-statefulset-experiment", "-n", "default",
					"-o", "jsonpath={.spec.template.spec.containers[0].image}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("nginx"))
			}, 1*time.Minute).Should(Succeed())

			// No need to clean up experiment as it's managed by Helm lifecycle
		})
	})
})
