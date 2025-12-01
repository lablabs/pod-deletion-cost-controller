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

	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lablabs/pod-deletion-cost-controller/test/utils"
)

// namespace where the project is deployed in
const namespace = "pod-deletion-cost-controller-system"

const timeout = time.Second * 30

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

			By(fmt.Sprintf("annotate deployment with: %q", cost.EnableAnnotation))
			annotateCmd := exec.Command(
				"kubectl", "annotate", "deployment", "app",
				fmt.Sprintf("%s=\"%s\"", cost.EnableAnnotation, "true"),
				"-n", namespace,
			)
			_, err = utils.Run(annotateCmd)
			Expect(err).NotTo(HaveOccurred())

			By("wait for pod to be running")
			waitCmd := exec.Command(
				"kubectl", "wait", "pods",
				"-l", "app=app",
				"-n", namespace,
				"--for=condition=Ready",
				fmt.Sprintf("--timeout=%s", timeout.String()),
			)
			_, err = utils.Run(waitCmd)

			By("list annotation for pod")
			annCmd := exec.Command(
				"kubectl", "get", "pods",
				"-n", namespace,
				"-o", fmt.Sprintf(`jsonpath={range .items[*]}{.metadata.name}={.metadata.annotations.%s}{"\n"}{end}`, strings.ReplaceAll(cost.PodDeletionCostAnnotation, ".", `\.`)),
			)
			out, err := utils.Run(annCmd)
			annotationsValue := make([]int, 0)
			pods := strings.Split(strings.TrimSpace(out), "\n")
			for _, pod := range pods {
				record := strings.Split(pod, "=")
				fmt.Fprintf(GinkgoWriter, "%s for pod %s = %s\n", cost.PodDeletionCostAnnotation, record[0], record[1])
				ct, err := strconv.Atoi(record[1])
				Expect(err).NotTo(HaveOccurred())
				annotationsValue = append(annotationsValue, ct)
			}
			Expect(len(annotationsValue)).Should(Equal(3))
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

			By(fmt.Sprintf("annotate deployment with: %q", cost.EnableAnnotation))
			annotateCmd := exec.Command(
				"kubectl", "annotate", "deployment", "app",
				fmt.Sprintf("%s=\"%s\"", cost.EnableAnnotation, "true"),
				"-n", namespace,
			)
			_, err = utils.Run(annotateCmd)
			Expect(err).NotTo(HaveOccurred())

			By("wait for pod to be running")
			waitCmd := exec.Command(
				"kubectl", "wait", "pods",
				"-l", "app=app",
				"-n", namespace,
				"--for=condition=Ready",
				fmt.Sprintf("--timeout=%s", timeout.String()),
			)
			_, err = utils.Run(waitCmd)
			Expect(err).NotTo(HaveOccurred())

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
				"--timeout=60s",
			)
			_, err = utils.Run(rolloutCmd)
			Expect(err).NotTo(HaveOccurred())

			By("wait for pod to be running")
			waitCmd = exec.Command(
				"kubectl", "wait", "pods",
				"-l", "app=app",
				"-n", namespace,
				"--for=condition=Ready",
				fmt.Sprintf("--timeout=%s", timeout.String()),
			)
			_, err = utils.Run(waitCmd)
			Expect(err).NotTo(HaveOccurred())

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

			By("list annotation for pod")
			annCmd := exec.Command(
				"kubectl", "get", "pods",
				"-l", "app=app,pod-template-hash="+strings.Trim(hash, "'"),
				"-n", namespace,
				"-o", fmt.Sprintf(`jsonpath={range .items[*]}{.metadata.name}={.metadata.annotations.%s}{"\n"}{end}`, strings.ReplaceAll(cost.PodDeletionCostAnnotation, ".", `\.`)),
			)
			out, err := utils.Run(annCmd)
			annotationsValue := make([]int, 0)
			pods := strings.Split(strings.TrimSpace(out), "\n")
			for _, pod := range pods {
				record := strings.Split(pod, "=")
				fmt.Fprintf(GinkgoWriter, "%s for pod %s = %s\n", cost.PodDeletionCostAnnotation, record[0], record[1])
				ct, err := strconv.Atoi(record[1])
				Expect(err).NotTo(HaveOccurred())
				annotationsValue = append(annotationsValue, ct)
			}
			Expect(len(annotationsValue)).To(Equal(3))

		})
	})

	// +kubebuilder:scaffold:e2e-webhooks-checks
})
