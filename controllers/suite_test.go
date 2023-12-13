/*
Copyright 2022-2023 Red Hat, Inc.

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

package controllers

import (
	"context"
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"

	appstudioredhatcomv1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	log       logr.Logger
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())
	log = ctrl.Log.WithName("testdebug")

	By("bootstrapping test environment")

	// Envtest doesn't respect kustomization.yaml for CRDs, apply it ourselves
	crdsTempfile, err := os.CreateTemp("", "crds*.yaml")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(crdsTempfile.Name())
	Expect(runKustomize(filepath.Join("..", "config", "crd"), crdsTempfile)).To(Succeed())
	crdsTempfile.Close()

	applicationServiceDepVersion := "v0.0.0-20230717184417-67d31a01a776"
	applicationApiDepVersion := "v0.0.0-20231026192857-89515ad2504f"
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			crdsTempfile.Name(),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "redhat-appstudio", "application-api@"+applicationApiDepVersion, "config", "crd", "bases"),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "redhat-appstudio", "application-service@"+applicationServiceDepVersion, "hack", "routecrd", "route.yaml"),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "tektoncd", "pipeline@v0.50.5", "config"),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "openshift-pipelines", "pipelines-as-code@v0.18.0", "config"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	cfg.Timeout = 5 * time.Second
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = routev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = appstudiov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = tektonapi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = pacv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = appstudioredhatcomv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	defaultNS := &corev1.Namespace{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: HASAppNamespace}, defaultNS)).Should(Succeed())
	defaultNS.SetLabels(map[string]string{
		appstudioWorkspaceNameLabel: "build",
	})
	Expect(k8sClient.Update(ctx, defaultNS)).Should(Succeed())

	svcAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildPipelineServiceAccountName,
			Namespace: "default",
		},
	}

	Expect(k8sClient.Create(context.Background(), &svcAccount)).Should(Succeed())

	Expect(patchPipelineRunCRD()).Should(Succeed())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&ComponentBuildReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: k8sManager.GetEventRecorderFor("ComponentOnboarding"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&PaCPipelineRunPrunerReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: k8sManager.GetEventRecorderFor("PaCPipelineRunPruner"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&GitTektonResourcesRenovater{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: k8sManager.GetEventRecorderFor("GitTektonResourcesRenovater"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

func runKustomize(dir string, out io.Writer) error {
	kubectl := filepath.Join(os.Getenv("KUBEBUILDER_ASSETS"), "kubectl")
	cmd := exec.Command(kubectl, "kustomize", dir)
	cmd.Stdout = out
	return cmd.Run()
}

// The Tekton PipelineRun CRD defines v1beta1 as the storage version. When build-service creates
// a v1 PipelineRun, it needs to be converted to v1beta1 before it gets stored in etcd. Normally,
// this would be done by a conversion webhook, but the webhook doesn't seem to work in the envtest
// environment.
//
// Adding the webhook path to envtest.Environment.WebhookInstallOptions does not help.
//
// Instead, patch the PipelineRun CRD. Set v1 as the storage version so that the conversion webhook
// is not needed.
//
// TODO: in github.com/tektoncd/pipelines@v0.49.0, the storage version changed to v1. After we
// update the dependency, we can drop this workaround.
// https://github.com/tektoncd/pipeline/commit/7384a67b77c07444f0e1d5748e771c1477e0db23
func patchPipelineRunCRD() error {
	var pipelineRunCRD apiextensionsv1.CustomResourceDefinition
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "", Name: "pipelineruns.tekton.dev"}, &pipelineRunCRD)
	if err != nil {
		return err
	}

	v1beta1 := &pipelineRunCRD.Spec.Versions[0]
	v1 := &pipelineRunCRD.Spec.Versions[1]

	Expect(v1beta1.Name).To(Equal("v1beta1"))
	Expect(v1.Name).To(Equal("v1"))

	v1beta1.Storage = false
	v1.Storage = true

	err = k8sClient.Update(ctx, &pipelineRunCRD)
	return err
}

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
