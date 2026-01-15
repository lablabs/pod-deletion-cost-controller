//go:build e2e
// +build e2e

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
	"strconv"
	"strings"
	"time"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lablabs/pod-deletion-cost-controller/test/utils"
)

// namespace where the project is deployed in
const namespace = "pod-deletion-cost-controller-system"

const timeout = time.Second * 120

var _ = Describe("Controller", Ordered, func() {
	BeforeAll(func() {
		By("deploy the controller via helm install") //my
		cmd := exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})
	AfterAll(func() {
		By("undeploy the controller via helm uninstall")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)
	})
	BeforeEach(func() {
		By("create test namespace")
		nsCmd := exec.Command("kubectl", "create", "namespace", namespace)
		_, err := utils.Run(nsCmd)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		By("delete test namespace")
		nsCmd := exec.Command("kubectl", "delete", "namespace", namespace)
		_, err := utils.Run(nsCmd)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("in Reconcile", func() {
		It("should add pod-deletion-cost annotation into Pod running under Deployment", func() {
			By("create deployment")
			deployCmd := exec.Command(
				"kubectl", "create", "deploy", "app",
				"--image=nginx",
				"--replicas=3",
				"-n", namespace,
			)
			_, err := utils.Run(deployCmd)
			Expect(err).NotTo(HaveOccurred())

			By(fmt.Sprintf("annotate deployment with: %q", controller.EnableAnnotation))
			annotateCmd := exec.Command(
				"kubectl", "annotate", "deployment", "app",
				fmt.Sprintf("%s=%s", controller.EnableAnnotation, "true"),
				"-n", namespace,
			)
			_, err = utils.Run(annotateCmd)
			Expect(err).NotTo(HaveOccurred())

			By("wait for pod to be running")
			Eventually(func() error {
				waitCmd := exec.Command(
					"kubectl", "wait", "pods",
					"-l", "app=app",
					"-n", namespace,
					"--for=condition=Ready",
					fmt.Sprintf("--timeout=%s", "0"),
				)
				out, err := utils.Run(waitCmd)
				fmt.Fprintf(GinkgoWriter, "%s", out)
				return err
			}, timeout.String(), "10s").Should(Succeed())

			By("list annotation for pod")
			Eventually(func() error {
				annCmd := exec.Command(
					"kubectl", "get", "pods",
					"-l", "app=app",
					"-n", namespace,
					"-o", fmt.Sprintf(`jsonpath={range .items[*]}{.metadata.annotations.%s}{"\n"}{end}`, strings.ReplaceAll(controller.PodDeletionCostAnnotation, ".", `\.`)),
				)
				out, err := utils.Run(annCmd)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(GinkgoWriter, "%s\n", out)
				Expect(err).NotTo(HaveOccurred())
				annValues := strings.Split(strings.TrimSpace(out), "\n")
				for _, cost := range annValues {
					_, err := strconv.Atoi(cost)
					if err != nil {
						return err
					}
				}
				return nil
			}, timeout.String(), "2s").Should(Succeed())
		})

		It("should add new interval of pod-deletion-cost annotation for replicaset", func() {
			By("create deployment")
			deployCmd := exec.Command(
				"kubectl", "create", "deploy", "app",
				"--image=nginx",
				"--replicas=3",
				"-n", namespace,
			)
			_, err := utils.Run(deployCmd)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				waitCmd := exec.Command(
					"kubectl", "wait", "pods",
					"-l", "app=app",
					"-n", namespace,
					"--for=condition=Ready",
					fmt.Sprintf("--timeout=%s", "0"),
				)
				_, err = utils.Run(waitCmd)
				return err
			}, timeout.String(), "2s").Should(Succeed())

			By(fmt.Sprintf("annotate deployment with: %q", controller.EnableAnnotation))
			annotateCmd := exec.Command(
				"kubectl", "annotate", "deployment", "app",
				fmt.Sprintf("%s=%s", controller.EnableAnnotation, "true"),
				"-n", namespace,
			)
			_, err = utils.Run(annotateCmd)
			Expect(err).NotTo(HaveOccurred())

			By("wait for pod to be running")
			Eventually(func() error {
				waitCmd := exec.Command(
					"kubectl", "wait", "pods",
					"-l", "app=app",
					"-n", namespace,
					"--for=condition=Ready",
					fmt.Sprintf("--timeout=%s", "0"),
				)
				_, err = utils.Run(waitCmd)
				return err
			}, timeout.String(), "2s").Should(Succeed())

			By("change env variable to trigger new ReplicaSet rollout")
			chaneEnvCmd := exec.Command(
				"kubectl", "set", "env",
				"deployment/app",
				"ENV=value",
				"-n", namespace,
			)
			_, err = utils.Run(chaneEnvCmd)
			Expect(err).NotTo(HaveOccurred())

			By("checking if new rs is rollout")
			rolloutCmd := exec.Command(
				"kubectl", "rollout", "status",
				"deployment/app",
				"-n", namespace,
				"--timeout=120s",
			)
			_, err = utils.Run(rolloutCmd)
			Expect(err).NotTo(HaveOccurred())

			By("wait for pod to be running")
			Eventually(func() error {
				waitCmd := exec.Command(
					"kubectl", "wait", "pods",
					"-l", "app=app",
					"-n", namespace,
					"--for=condition=Ready",
					fmt.Sprintf("--timeout=%s", "0"),
				)
				_, err = utils.Run(waitCmd)
				return err
			}, timeout.String(), "2s").Should(Succeed())

			By("getting latest rs")
			rsNameCmd := exec.Command(
				"kubectl", "get", "rs",
				"-l", "app=app",
				"-n", namespace,
				"--sort-by=.metadata.creationTimestamp",
				"-o", "jsonpath='{.items[-1:].metadata.name}'",
			)
			rsName, err := utils.Run(rsNameCmd)
			Expect(err).NotTo(HaveOccurred())

			By("getting pod hash from rs" + strings.Trim(rsName, "'"))
			hashCmd := exec.Command(
				"kubectl", "get", "rs",
				strings.Trim(rsName, "'"),
				"-n", namespace,
				"-o", "jsonpath='{.metadata.labels.pod-template-hash}'",
			)
			hash, err := utils.Run(hashCmd)
			Expect(err).NotTo(HaveOccurred())

			By("list annotation for pods")
			Eventually(func() error {
				annCmd := exec.Command(
					"kubectl", "get", "pods",
					"-l", "app=app,pod-template-hash="+strings.Trim(hash, "'"),
					"-n", namespace,
					"-o", fmt.Sprintf(`jsonpath={range .items[*]}{.metadata.annotations.%s}{"\n"}{end}`, strings.ReplaceAll(controller.PodDeletionCostAnnotation, ".", `\.`)),
				)
				out, err := utils.Run(annCmd)
				if err != nil {
					return err
				}
				fmt.Fprintf(GinkgoWriter, "%s\n", out)
				annValues := strings.Split(strings.TrimSpace(out), "\n")
				for _, cost := range annValues {
					_, err := strconv.Atoi(cost)
					if err != nil {
						return err
					}
				}
				return nil
			}, timeout.String(), "2s").Should(Succeed())
		})
	})

	// +kubebuilder:scaffold:e2e-webhooks-checks
})
