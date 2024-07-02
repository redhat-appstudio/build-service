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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	. "github.com/konflux-ci/build-service/pkg/common"
	"gopkg.in/yaml.v2"
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
	ComponentContainerImage = "registry.io/username/image:tag"
	SelectorDefaultName     = "default"

	defaultPipelineName   = "docker-build"
	defaultPipelineBundle = "quay.io/redhat-appstudio-tekton-catalog/pipeline-docker-build:07ec767c565b36296b4e185b01f05536848d9c12"
)

var (
	defaultPipelineConfigMapKey = types.NamespacedName{Name: buildPipelineConfigMapResourceName, Namespace: BuildServiceNamespaceName}
)

type componentConfig struct {
	componentKey     types.NamespacedName
	containerImage   string
	gitURL           string
	gitRevision      string
	gitSourceContext string
	application      string
	annotations      map[string]string
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
		gitUrl = SampleRepoLink + "-" + name
	}
	gitRevision := config.gitRevision
	if gitRevision == "" {
		gitRevision = "main"
	}
	application := config.application
	if application == "" {
		application = HASAppName
	}
	annotations := make(map[string]string)
	if config.annotations != nil {
		for key, value := range config.annotations {
			annotations[key] = value
		}
	}
	return &appstudiov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
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
						Context:  config.gitSourceContext,
					},
				},
			},
		},
	}
}

func getSampleComponentData(componentKey types.NamespacedName) *appstudiov1alpha1.Component {
	return getComponentData(componentConfig{componentKey: componentKey})
}

// createComponentAndProcessBuildRequest create a component with specified build request and
// waits until the request annotation is removed, which means the request was processed by the operator.
// Use createCustomComponentWithBuildRequest if there is no need to wait.
func createComponentAndProcessBuildRequest(config componentConfig, buildRequest string) {
	createCustomComponentWithBuildRequest(config, buildRequest)
	waitComponentAnnotationGone(config.componentKey, BuildRequestAnnotationName)
}

func createCustomComponentWithoutBuildRequest(config componentConfig) {
	component := getComponentData(config)
	if component.Annotations == nil {
		component.Annotations = make(map[string]string)
	}

	Expect(k8sClient.Create(ctx, component)).Should(Succeed())
	getComponent(config.componentKey)
}

func createCustomComponentWithBuildRequest(config componentConfig, buildRequest string) {
	component := getComponentData(config)
	if component.Annotations == nil {
		component.Annotations = make(map[string]string)
	}
	component.Annotations[BuildRequestAnnotationName] = buildRequest

	Expect(k8sClient.Create(ctx, component)).Should(Succeed())
	getComponent(config.componentKey)
}

func setComponentBuildRequest(componentKey types.NamespacedName, buildRequest string) {
	component := getComponent(componentKey)
	if component.Annotations == nil {
		component.Annotations = make(map[string]string)
	}
	component.Annotations[BuildRequestAnnotationName] = buildRequest

	Expect(k8sClient.Update(ctx, component)).To(Succeed())
}

// createComponent creates sample component resource and verifies it was properly created
func createComponent(componentLookupKey types.NamespacedName) {
	component := getSampleComponentData(componentLookupKey)

	createComponentCustom(component)
}

// createComponentCustom creates custom component resource and verifies it was properly created
func createComponentCustom(sampleComponentData *appstudiov1alpha1.Component) {
	Expect(k8sClient.Create(ctx, sampleComponentData)).Should(Succeed())

	lookupKey := types.NamespacedName{
		Name:      sampleComponentData.Name,
		Namespace: sampleComponentData.Namespace,
	}

	getComponent(lookupKey)
}

func getComponent(componentKey types.NamespacedName) *appstudiov1alpha1.Component {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, componentKey, component); err != nil {
			return false
		}
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
		if err := k8sClient.Get(ctx, componentKey, component); err != nil {
			return err
		}
		return k8sClient.Delete(ctx, component)
	}, timeout, interval).Should(Succeed())

	// Wait for delete to finish
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, componentKey, component))
	}, timeout, interval).Should(BeTrue())
}

func waitFinalizerOnComponent(componentKey types.NamespacedName, finalizerName string, finalizerShouldBePresent bool) {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, componentKey, component); err != nil {
			return false
		}

		if finalizerShouldBePresent {
			return controllerutil.ContainsFinalizer(component, finalizerName)
		} else {
			return !controllerutil.ContainsFinalizer(component, finalizerName)
		}
	}, timeout, interval).Should(BeTrue())
}

func waitPaCFinalizerOnComponent(componentKey types.NamespacedName) {
	waitFinalizerOnComponent(componentKey, PaCProvisionFinalizer, true)
}

func waitPaCFinalizerOnComponentGone(componentKey types.NamespacedName) {
	waitFinalizerOnComponent(componentKey, PaCProvisionFinalizer, false)
}

func waitDoneMessageOnComponent(componentKey types.NamespacedName) {
	Eventually(func() bool {
		buildStatus := readBuildStatus(getComponent(componentKey))
		return buildStatus.Message == "done"
	}, timeout, interval).Should(BeTrue())
}

func expectPacBuildStatus(componentKey types.NamespacedName, state string, errID int, errMessage string, mergeURL string) {
	// in 1 test component is usually created (which triggers reconcile and adds message=done)
	// and then component is updated (and waits for message=done apart from other things)
	// we should wait for desired state as well, as there is small time window when
	// message might be still done from previous reconcile, but even though requested action already finished,
	// it didn't update status yet so it will get status from previous reconcile
	Eventually(func() bool {
		buildStatus := readBuildStatus(getComponent(componentKey))
		Expect(buildStatus).ToNot(BeNil())
		Expect(buildStatus.PaC).ToNot(BeNil())

		return buildStatus.PaC.State == state
	}, timeout, interval).Should(BeTrue())

	buildStatus := readBuildStatus(getComponent(componentKey))
	Expect(buildStatus).ToNot(BeNil())
	Expect(buildStatus.PaC).ToNot(BeNil())
	Expect(buildStatus.PaC.State).To(Equal(state))
	Expect(buildStatus.PaC.ErrId).To(Equal(errID))
	Expect(buildStatus.PaC.ErrMessage).To(Equal(errMessage))
	Expect(buildStatus.PaC.MergeUrl).To(Equal(mergeURL))
	if state == "enabled" {
		Expect(buildStatus.PaC.ConfigurationTime).ToNot(BeEmpty())
	} else {
		Expect(buildStatus.PaC.ConfigurationTime).To(BeEmpty())
	}
}

func expectSimpleBuildStatus(componentKey types.NamespacedName, errID int, errMessage string, startTimeEmpty bool) {
	buildStatus := readBuildStatus(getComponent(componentKey))
	Expect(buildStatus).ToNot(BeNil())
	Expect(buildStatus.Simple).ToNot(BeNil())
	Expect(buildStatus.Simple.ErrId).To(Equal(errID))
	Expect(buildStatus.Simple.ErrMessage).To(Equal(errMessage))
	if startTimeEmpty {
		Expect(buildStatus.Simple.BuildStartTime).To(Equal(""))
	} else {
		Expect(buildStatus.Simple.BuildStartTime).ToNot(BeEmpty())
	}
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

	Eventually(func() error {
		secret := &corev1.Secret{}
		return k8sClient.Get(ctx, resourceKey, secret)
	}, timeout, interval).Should(Succeed())
}

func createSCMSecret(resourceKey types.NamespacedName, data map[string]string, secretType corev1.SecretType, annotations map[string]string) {
	labels := map[string]string{
		"appstudio.redhat.com/credentials": "scm",
		"appstudio.redhat.com/scm.host":    "github.com",
	}
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		Type: secretType,
		ObjectMeta: metav1.ObjectMeta{
			Name:        resourceKey.Name,
			Namespace:   resourceKey.Namespace,
			Labels:      labels,
			Annotations: annotations,
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

	Eventually(func() error {
		secret := &corev1.Secret{}
		return k8sClient.Get(ctx, resourceKey, secret)
	}, timeout, interval).Should(Succeed())
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

func waitSecretGone(resourceKey types.NamespacedName) {
	secret := &corev1.Secret{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, resourceKey, secret)
		return k8sErrors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
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

func waitPaCRepositoryCreated(resourceKey types.NamespacedName) *pacv1alpha1.Repository {
	pacRepository := &pacv1alpha1.Repository{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, resourceKey, pacRepository)
		return err == nil && pacRepository.ResourceVersion != ""
	}, timeout, interval).Should(BeTrue())
	return pacRepository
}

func deletePaCRepository(resourceKey types.NamespacedName) {
	pacRepository := &pacv1alpha1.Repository{}
	if err := k8sClient.Get(ctx, resourceKey, pacRepository); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, pacRepository); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, pacRepository))
	}, timeout, interval).Should(BeTrue())
}

func deleteAllPaCRepositories(namesapce string) {
	opts := &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			Namespace: namesapce,
		},
	}
	Expect(k8sClient.DeleteAllOf(ctx, &pacv1alpha1.Repository{}, opts)).To(Succeed())
}

func waitComponentAnnotationGone(componentKey types.NamespacedName, annotationName string) {
	Eventually(func() bool {
		component := getComponent(componentKey)
		annotations := component.GetAnnotations()
		if annotations == nil {
			return true
		}
		_, exists := annotations[annotationName]
		return !exists
	}, timeout, interval).Should(BeTrue())
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

func createDefaultBuildPipelineConfigMap(configMapKey types.NamespacedName) {
	createBuildPipelineConfigMap(configMapKey, defaultPipelineBundle, defaultPipelineName)
}

func createBuildPipelineConfigMap(configMapKey types.NamespacedName, pipelineBundle, pipelineName string) {
	configMapData := map[string]string{}
	buildPipelineData := pipelineConfig{
		DefaultPipelineName: pipelineName,
		Pipelines:           []BuildPipeline{{Name: pipelineName, Bundle: pipelineBundle}},
	}
	yamlData, _ := yaml.Marshal(&buildPipelineData)
	configMapData[buildPipelineConfigName] = string(yamlData)

	buildPipelineConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: configMapKey.Name, Namespace: configMapKey.Namespace},
		Data:       configMapData,
	}

	if err := k8sClient.Create(ctx, &buildPipelineConfigMap); err != nil && !k8sErrors.IsAlreadyExists(err) {
		Fail(err.Error())
	}
}

func deleteBuildPipelineConfigMap(configMapKey types.NamespacedName) {
	buildPipelineConfigMap := corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, configMapKey, &buildPipelineConfigMap); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, &buildPipelineConfigMap); err != nil && !k8sErrors.IsNotFound(err) {
		Fail(err.Error())
	}
}

func getPipelineName(pipelineRef *tektonapi.PipelineRef) string {
	name, _, _ := getPipelineNameAndBundle(pipelineRef)
	return name
}

func getPipelineBundle(pipelineRef *tektonapi.PipelineRef) string {
	_, bundle, _ := getPipelineNameAndBundle(pipelineRef)
	return bundle
}
