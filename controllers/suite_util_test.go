/*
Copyright 2022.

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
	"fmt"
	"math/rand"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	gh "github.com/google/go-github/v45/github"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	buildappstudiov1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	"github.com/redhat-appstudio/build-service/pkg/github"
)

const (
	// timeout is used as a limit until condition become true
	// Usually used in Eventually statements
	timeout = time.Second * 15
	// ensureTimeout is used as a period of time during which the condition should not be changed
	// Usually used in Consistently statements
	ensureTimeout = time.Second * 4
	interval      = time.Millisecond * 250
)

const (
	HASAppName              = "test-application"
	HASCompName             = "test-component"
	HASAppNamespace         = "default"
	SampleRepoLink          = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	GitSecretName           = "git-secret"
	ComponentContainerImage = "registry.io/username/image:tag"
	SelectorDefaultName     = "default"
)

type componentConfig struct {
	componentKey   types.NamespacedName
	containerImage string
	gitURL         string
	gitRevision    string
	application    string
}

func isOwnedBy(resource []metav1.OwnerReference, component appstudiov1alpha1.Component) bool {
	if len(resource) == 0 {
		return false
	}
	if resource[0].Kind == "Component" &&
		resource[0].APIVersion == "appstudio.redhat.com/v1alpha1" &&
		resource[0].Name == component.Name {
		return true
	}
	return false
}

func getMinimalDevfile() string {
	return `
        schemaVersion: 2.2.0
        metadata:
            name: minimal-devfile
    `
}

func getComponentData(config componentConfig) *appstudiov1alpha1.Component {
	name := config.componentKey.Name
	if name == "" {
		name = HASCompName
	}
	namespace := config.componentKey.Namespace
	if namespace == "" {
		namespace = HASAppNamespace
	}
	image := config.containerImage
	if image == "" {
		image = ComponentContainerImage
	}
	gitUrl := config.gitURL
	if gitUrl == "" {
		gitUrl = SampleRepoLink
	}
	gitRevision := config.gitRevision
	if gitRevision == "" {
		gitRevision = "main"
	}
	application := config.application
	if application == "" {
		application = HASAppName
	}
	return &appstudiov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			ComponentName:  name,
			Application:    application,
			ContainerImage: image,
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL:      gitUrl,
						Revision: gitRevision,
					},
				},
			},
		},
	}
}

func getSampleComponentData(componentKey types.NamespacedName) *appstudiov1alpha1.Component {
	return getComponentData(componentConfig{componentKey: componentKey})
}

// createComponent creates sample component resource and verifies it was properly created
func createComponentForPaCBuild(sampleComponentData *appstudiov1alpha1.Component) types.NamespacedName {
	sampleComponentData.Annotations = map[string]string{
		PaCProvisionAnnotationName: PaCProvisionRequestedAnnotationValue,
	}

	Expect(k8sClient.Create(ctx, sampleComponentData)).Should(Succeed())

	lookupKey := types.NamespacedName{
		Name:      sampleComponentData.Name,
		Namespace: sampleComponentData.Namespace,
	}
	getComponent(lookupKey)
	return lookupKey
}

// createComponent creates sample component resource and verifies it was properly created
func createComponent(componentLookupKey types.NamespacedName) {
	component := getSampleComponentData(componentLookupKey)

	Expect(k8sClient.Create(ctx, component)).Should(Succeed())

	getComponent(componentLookupKey)
}

func getComponent(componentKey types.NamespacedName) *appstudiov1alpha1.Component {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() bool {
		Expect(k8sClient.Get(ctx, componentKey, component)).Should(Succeed())
		return component.ResourceVersion != ""
	}, timeout, interval).Should(BeTrue())
	return component
}

// deleteComponent deletes the specified component resource and verifies it was properly deleted
func deleteComponent(componentKey types.NamespacedName) {
	component := &appstudiov1alpha1.Component{}

	// Check if the component exists
	if err := k8sClient.Get(ctx, componentKey, component); k8sErrors.IsNotFound(err) {
		return
	}

	// Delete
	Eventually(func() error {
		Expect(k8sClient.Get(ctx, componentKey, component)).To(Succeed())
		return k8sClient.Delete(ctx, component)
	}, timeout, interval).Should(Succeed())

	// Wait for delete to finish
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, componentKey, component))
	}, timeout, interval).Should(BeTrue())
}

func setComponentDevfile(componentKey types.NamespacedName, devfile string) {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() error {
		if err := k8sClient.Get(ctx, componentKey, component); err != nil {
			return err
		}
		component.Status.Devfile = devfile
		return k8sClient.Status().Update(ctx, component)
	}, timeout, interval).Should(Succeed())

	component = getComponent(componentKey)
	Expect(component.Status.Devfile).Should(Not(Equal("")))
}

func setComponentDevfileModel(componentKey types.NamespacedName) {
	devfile := getMinimalDevfile()
	setComponentDevfile(componentKey, devfile)
}

func waitPaCFinalizerOnComponent(componentKey types.NamespacedName) {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, componentKey, component); err != nil {
			return false
		}
		return controllerutil.ContainsFinalizer(component, PaCProvisionFinalizer)
	}, timeout, interval).Should(BeTrue())
}

func listComponentPipelineRuns(componentKey types.NamespacedName) []tektonapi.PipelineRun {
	pipelineRuns := &tektonapi.PipelineRunList{}
	labelSelectors := client.ListOptions{
		Raw:       &metav1.ListOptions{LabelSelector: ComponentNameLabelName + "=" + componentKey.Name},
		Namespace: componentKey.Namespace,
	}
	err := k8sClient.List(ctx, pipelineRuns, &labelSelectors)
	Expect(err).ToNot(HaveOccurred())
	return pipelineRuns.Items
}

func deleteComponentPipelineRuns(componentKey types.NamespacedName) {
	for _, pipelineRun := range listComponentPipelineRuns(componentKey) {
		if err := k8sClient.Delete(ctx, &pipelineRun); err != nil {
			if !k8sErrors.IsNotFound(err) {
				Fail(err.Error())
			}
		}
	}
}

func createPaCPipelineRunWithName(resourceKey types.NamespacedName, pipelineRunName string) {
	pipelineRun := &tektonapi.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pipelineRunName,
			Namespace: resourceKey.Namespace,
			Labels: map[string]string{
				ComponentNameLabelName: resourceKey.Name,
			},
		},
	}
	Expect(k8sClient.Create(ctx, pipelineRun)).Should(Succeed())

	pipelineRunKey := types.NamespacedName{Namespace: pipelineRun.Namespace, Name: pipelineRun.Name}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, pipelineRunKey, pipelineRun)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

func waitOneInitialPipelineRunCreated(componentKey types.NamespacedName) {
	component := getComponent(componentKey)
	Eventually(func() bool {
		pipelineRuns := listComponentPipelineRuns(componentKey)
		if len(pipelineRuns) != 1 {
			return false
		}
		pipelineRun := pipelineRuns[0]
		return isOwnedBy(pipelineRun.OwnerReferences, *component)
	}, timeout, interval).Should(BeTrue(),
		fmt.Sprintf("No pipelinerun is created for component: %v", component))
}

func ensureNoPipelineRunsCreated(componentLookupKey types.NamespacedName) {
	pipelineRuns := &tektonapi.PipelineRunList{}
	Consistently(func() bool {
		labelSelectors := client.ListOptions{
			Raw:       &metav1.ListOptions{LabelSelector: ComponentNameLabelName + "=" + componentLookupKey.Name},
			Namespace: componentLookupKey.Namespace,
		}
		Expect(k8sClient.List(ctx, pipelineRuns, &labelSelectors)).To(Succeed())
		return len(pipelineRuns.Items) == 0
	}, timeout, interval).WithTimeout(ensureTimeout).Should(BeTrue())
}

func waitNoPipelineRunsForComponent(componentLookupKey types.NamespacedName) {
	pipelineRuns := &tektonapi.PipelineRunList{}
	Eventually(func() bool {
		labelSelectors := client.ListOptions{
			Raw:       &metav1.ListOptions{LabelSelector: ComponentNameLabelName + "=" + componentLookupKey.Name},
			Namespace: componentLookupKey.Namespace,
		}
		Expect(k8sClient.List(ctx, pipelineRuns, &labelSelectors)).To(Succeed())
		return len(pipelineRuns.Items) == 0
	}, timeout, interval).WithTimeout(ensureTimeout).Should(BeTrue())
}

func createSecret(resourceKey types.NamespacedName, data map[string]string) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
		StringData: data,
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		if !k8sErrors.IsAlreadyExists(err) {
			Fail(err.Error())
		}
		deleteSecret(resourceKey)
		secret.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	}
}

func deleteSecret(resourceKey types.NamespacedName) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, resourceKey, secret); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, secret); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, secret))
	}, timeout, interval).Should(BeTrue())
}

func waitSecretCreated(resourceKey types.NamespacedName) {
	secret := &corev1.Secret{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, resourceKey, secret)
		return err == nil && secret.ResourceVersion != ""
	}, timeout, interval).Should(BeTrue())
}

func ensureSecretNotCreated(resourceKey types.NamespacedName) {
	secret := &corev1.Secret{}
	Consistently(func() bool {
		err := k8sClient.Get(ctx, resourceKey, secret)
		return k8sErrors.IsNotFound(err)
	}, timeout, interval).WithTimeout(ensureTimeout).Should(BeTrue())
}

func createNamespace(name string) {
	namespace := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := k8sClient.Create(ctx, &namespace); err != nil && !k8sErrors.IsAlreadyExists(err) {
		Fail(err.Error())
	}
}

func deleteNamespace(name string) {
	namespace := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := k8sClient.Delete(ctx, &namespace); err != nil && !k8sErrors.IsNotFound(err) {
		Fail(err.Error())
	}
}

func waitPaCRepositoryCreated(resourceKey types.NamespacedName) {
	pacRepository := &pacv1alpha1.Repository{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, resourceKey, pacRepository)
		return err == nil && pacRepository.ResourceVersion != ""
	}, timeout, interval).Should(BeTrue())
}

func waitComponentAnnotationValue(componentKey types.NamespacedName, annotationName string, annotationValue string) {
	Eventually(func() bool {
		component := getComponent(componentKey)
		annotations := component.GetAnnotations()
		return annotations != nil && annotations[annotationName] == annotationValue
	}, timeout, interval).Should(BeTrue())
}

func ensureComponentAnnotationValue(componentKey types.NamespacedName, annotationName string, annotationValue string) {
	Consistently(func() bool {
		component := getComponent(componentKey)
		annotations := component.GetAnnotations()
		return annotations != nil && annotations[annotationName] == annotationValue
	}, ensureTimeout, interval).Should(BeTrue())
}

func ensureComponentInitialBuildAnnotationState(componentKey types.NamespacedName, initialBuildAnnotation bool) {
	if initialBuildAnnotation {
		Eventually(func() bool {
			component := getComponent(componentKey)
			annotations := component.GetAnnotations()
			return annotations != nil && annotations[InitialBuildAnnotationName] != ""
		}, timeout, interval).Should(BeTrue())
	} else {
		Consistently(func() bool {
			component := getComponent(componentKey)
			annotations := component.GetAnnotations()
			if annotations == nil {
				return true
			}
			_, exists := annotations[InitialBuildAnnotationName]
			return !exists
		}, timeout, interval).WithTimeout(ensureTimeout).Should(BeTrue())
	}
}

func createRoute(routeKey types.NamespacedName, host string) {
	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeKey.Name,
			Namespace: routeKey.Namespace,
		},
		Spec: routev1.RouteSpec{
			Host: host,
		},
	}

	if err := k8sClient.Create(ctx, &route); err != nil && !k8sErrors.IsAlreadyExists(err) {
		Fail(err.Error())
	}
}

func deleteRoute(routeKey types.NamespacedName) {
	route := &routev1.Route{}
	if err := k8sClient.Get(ctx, routeKey, route); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, route); err != nil && !k8sErrors.IsNotFound(err) {
		Fail(err.Error())
	}
}

func createBuildPipelineRunSelector(selectorKey types.NamespacedName) {
	buildPipelineSelector := buildappstudiov1alpha1.BuildPipelineSelector{
		ObjectMeta: metav1.ObjectMeta{Name: selectorKey.Name, Namespace: selectorKey.Namespace},
		Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
			Selectors: []buildappstudiov1alpha1.PipelineSelector{
				{
					Name:           SelectorDefaultName,
					PipelineRef:    tektonapi.PipelineRef{},
					PipelineParams: []buildappstudiov1alpha1.PipelineParam{},
					WhenConditions: buildappstudiov1alpha1.WhenCondition{},
				}}},
	}

	if err := k8sClient.Create(ctx, &buildPipelineSelector); err != nil && !k8sErrors.IsAlreadyExists(err) {
		Fail(err.Error())
	}
}
func deleteBuildPipelineRunSelector(selectorKey types.NamespacedName) {
	buildPipelineSelector := buildappstudiov1alpha1.BuildPipelineSelector{}
	if err := k8sClient.Get(ctx, selectorKey, &buildPipelineSelector); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, &buildPipelineSelector); err != nil && !k8sErrors.IsNotFound(err) {
		Fail(err.Error())
	}
}

func listJobs(namespace string) []batch.Job {
	jobs := &batch.JobList{}

	err := k8sClient.List(ctx, jobs, client.InNamespace(namespace))
	Expect(err).ToNot(HaveOccurred())
	return jobs.Items
}

func deleteJobs(namespace string) {
	err := k8sClient.DeleteAllOf(ctx, &batch.Job{}, client.InNamespace(namespace), client.PropagationPolicy(metav1.DeletePropagationBackground))
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		return len(listJobs(namespace)) == 0
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
}

func generateInstallation(repositories []*gh.Repository) github.ApplicationInstallation {
	return github.ApplicationInstallation{
		ID:           int64(rand.Intn(100)),
		Token:        getRandomString(30),
		Repositories: repositories,
	}
}

func generateRepository(repoURL string) *gh.Repository {
	repoURLParts := strings.Split(repoURL, "/")
	return &gh.Repository{
		HTMLURL:  &repoURL,
		FullName: gh.String(fmt.Sprintf("%s/%s", repoURLParts[3], repoURLParts[4])),
	}
}

func generateRepositories(repoURL []string) []*gh.Repository {
	repositories := []*gh.Repository{}
	for _, repo := range repoURL {
		repositories = append(repositories, generateRepository(repo))
	}
	return repositories
}
