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

package controller_test

import (
	"context"
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	"github.com/lablabs/pod-deletion-cost-controller/internal/zone"
	"github.com/lablabs/pod-deletion-cost-controller/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "PodDeletionCost Controller")
}

var _ = BeforeSuite(func() {
	log := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(log)

	ctx, cancel = context.WithCancel(context.Background())

	//
	// 1️⃣ Add ALL schemes BEFORE starting envtest
	//
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme.Scheme)).To(Succeed())
	// +kubebuilder:scaffold:scheme
	Expect(scheme.AddToScheme(scheme.Scheme)).To(Succeed())

	//
	// 2️⃣ Start test environment
	//
	binaryPath, err := utils.GetK8sBinaryDir(5, []string{"bin", "k8s"})
	Expect(err).NotTo(HaveOccurred())
	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: false,
		BinaryAssetsDirectory: binaryPath,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	//
	// 3️⃣ Create k8s client
	//
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	//
	// 4️⃣ Create manager
	//
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	moduleMng := controller.NewModuleManager()
	err = zone.Register(log, moduleMng, mgr.GetClient(), []string{})
	Expect(err).NotTo(HaveOccurred())
	err = (&controller.PodReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Manager: moduleMng,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	// 6️⃣ Start the manager in background
	//
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
