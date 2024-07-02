/*
Copyright 2021-2023 Red Hat, Inc.

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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/h2non/gock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/build-service/pkg/boerrors"
	. "github.com/konflux-ci/build-service/pkg/common"
	"github.com/konflux-ci/build-service/pkg/git/github"
	gp "github.com/konflux-ci/build-service/pkg/git/gitprovider"
	gpf "github.com/konflux-ci/build-service/pkg/git/gitproviderfactory"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	//+kubebuilder:scaffold:imports
)

const (
	githubAppPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAtSwZCtZ0Tnepuezo/TL9vhdP00fOedCpN3HsKjqz7zXTnqkq
foxemrDvRSg5n73sZhZMYX6NKY1FBLTE6OazvJQg0eXu7Bf5+5sg5ZZIthX1wbPU
wk18S5HsC1NTDGHVlQ2pQEuXmpxi/FAKgHaYbhx2J2SzD2gdKEOAufBZ14o8btF2
x64wi0VHX2JAmPXjodYQ2jYbH27lik5kS8wMDRKq0Kt3ABJkxULxUB4xuEbl2gWt
osDrhFTtDWEYtmtL0R5mLoK61FfZmZElvHELQObBcEpJu9F/Bi0mcjle/TuVc69e
tuVbmVg5Bn5iQKtw2hPEMqTLDVPbhhSAXhtJOwIDAQABAoIBAD+XEdce/MXJ9JXg
1MqCilOdZRRYoN1a4vomD2mnHx74OqX25IZ0iIQtVF5mxwsNo5sVeovB2pRaFH6Z
YIAK8c1gBMEHvru5krHAemR7Qlw/CvqJP0VP4y+3MS2senrfIBNoLx71KWpIN+ot
wfHjLo9/h+09yCfBOHK4dsdM2IvxTw9Md6ivipQC7rZmg0lpupNVRKwsx+VQoHCr
wzyPN14w+3tAJA2QoSdsqZLWkfY5YgxYmGuEG7L1A4Ynsjq6lEJyoclbZ6Gp+JML
G9MB0llXz9AV2k+BuNZWQO4cBPeKYtiQ1O5iYohghjSgPIx0iFa/GAvIrZscF2cf
Y43bN/ECgYEA3pKZ55L+oZIym0P2t9tqUaFhFewLdc54WhoMBrjKDQ7PL5n/avG5
Tac1RKA7AoknLYkqWilwRAzSBzNDjYsFVbfxRA10LagP61WhgGAPtEsDM3gbwuNw
HayYowutQ6422Ri4+ujWSeMMSrvx7RcHMPpr3hDSm8ihxbqsWkDNGQMCgYEA0GG/
cdtidDZ0aPwIqBdx6DHwKraYMt1fIzkw1qG59X7jc/Rym2aPUzDob99q8BieBRB3
6djzn+X97z40/87DRFACj/YQPkGvumSdRUtP88b0x87UhbHoOfEIkDeVYyxkbfl0
Mo6H9tlw+QSTaU13AzIBaWOB4nsQaKAM9isHrWkCgYAO0F8iBKyiAGMR5oIjVp1K
9ZzKor1Yh/eGt7kZMW9xUw0DNBLGAXS98GUhPjDvSEWtSDXjbmKkhN3t0MGsSBaA
0A9k4ihbaZY1qatoKfyhmWSLJnFilVS/BN/b6kkL+ip4ZKbbPGgW3t/QkZXWm/PE
lMZdL211JPNvf689CpccFQKBgGXJGUZ4LuMtJjeRxHi22wDcQ7/ZaQaPc0U1TlHI
tZjg3iFpqgGWWzP7k83xh763h5hZrvke7AGSyjLuY90AFglsO5QuUUjXtQqK0vdi
Di+5Yx+mO9ECUbjbr58iR2ol6Ph+/O8lB+zf0XsRbR/moteAuYfM/0itbBpu82Xb
JujhAoGAVdNMYdamHOJlkjYjYJmWOHPIFBMTDYk8pDATLOlqthV+fzlD2SlF0wxm
OlxbwEr3BSQlE3GFPHe+J3JV3BbJFsVkM8KdMUpQCl+aD/3nWaGBYHz4yvJc0i9h
duAFIZgXeivAZQAqys4JanG81cg4/d4urI3Qk9stlhB5CNCJR4k=
-----END RSA PRIVATE KEY-----`
)

var (
	defaultPipelineAnnotationValue = fmt.Sprintf("{\"name\":\"%s\",\"bundle\":\"%s\"}", defaultPipelineName, defaultPipelineBundle)
	defaultPipelineAnnotations     = map[string]string{defaultBuildPipelineAnnotation: defaultPipelineAnnotationValue}
)

var _ = Describe("Component initial build controller", func() {

	const (
		pacHost       = "pac-host"
		pacWebhookUrl = "https://" + pacHost
	)

	var (
		// All related to the component resources have the same key (but different type)
		pacRouteKey           = types.NamespacedName{Name: pipelinesAsCodeRouteName, Namespace: pipelinesAsCodeNamespace}
		pacSecretKey          = types.NamespacedName{Name: PipelinesAsCodeGitHubAppSecretName, Namespace: BuildServiceNamespaceName}
		namespacePaCSecretKey = types.NamespacedName{Name: PipelinesAsCodeGitHubAppSecretName, Namespace: HASAppNamespace}
		webhookSecretKey      = types.NamespacedName{Name: pipelinesAsCodeWebhooksSecretName, Namespace: HASAppNamespace}
	)

	BeforeEach(func() {
		createNamespace(BuildServiceNamespaceName)
		github.GetAllAppInstallations = func(githubAppIdStr string, appPrivateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
			return nil, "slug", nil
		}
	})

	Context("Test Pipelines as Code build preparation", func() {
		var resourcePacPrepKey = types.NamespacedName{Name: HASCompName + "-pacprepannotation", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(BuildServiceNamespaceName)
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			ResetTestGitProviderClient()
		})

		_ = AfterEach(func() {
			deleteComponent(resourcePacPrepKey)
			deletePaCRepository(resourcePacPrepKey)

			deleteSecret(webhookSecretKey)
			deleteSecret(namespacePaCSecretKey)

			deleteSecret(pacSecretKey)
			deleteRoute(pacRouteKey)
		})

		It("should successfully submit PR with PaC definitions using GitHub application", func() {
			mergeUrl := "merge-url"

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourcePacPrepKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).To(Equal("main"))
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return mergeUrl, nil
			}
			SetupPaCWebhookFunc = func(string, string, string) error {
				defer GinkgoRecover()
				Fail("Should not create webhook if GitHub application is used")
				return nil
			}

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			pacRepo := waitPaCRepositoryCreated(resourcePacPrepKey)
			Expect(pacRepo.Spec.Params).ShouldNot(BeNil())
			existingParams := map[string]string{}
			for _, param := range *pacRepo.Spec.Params {
				existingParams[param.Name] = param.Value
			}
			val, ok := existingParams[pacCustomParamAppstudioWorkspace]
			Expect(ok).Should(BeTrue())
			Expect(val).Should(Equal("build"))

			waitPaCFinalizerOnComponent(resourcePacPrepKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "enabled", 0, "", mergeUrl)
		})

		It("should successfully submit PR with PaC definitions using GitHub application, even when pipeline annotation is missing, will add default one", func() {
			mergeUrl := "merge-url"

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourcePacPrepKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).To(Equal("main"))
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return mergeUrl, nil
			}
			SetupPaCWebhookFunc = func(string, string, string) error {
				defer GinkgoRecover()
				Fail("Should not create webhook if GitHub application is used")
				return nil
			}

			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			annotations := map[string]string{}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  annotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			pacRepo := waitPaCRepositoryCreated(resourcePacPrepKey)
			Expect(pacRepo.Spec.Params).ShouldNot(BeNil())
			existingParams := map[string]string{}
			for _, param := range *pacRepo.Spec.Params {
				existingParams[param.Name] = param.Value
			}
			val, ok := existingParams[pacCustomParamAppstudioWorkspace]
			Expect(ok).Should(BeTrue())
			Expect(val).Should(Equal("build"))

			waitPaCFinalizerOnComponent(resourcePacPrepKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "enabled", 0, "", mergeUrl)
			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
		})

		It("should successfully submit PR with PaC definitions using GitHub application, using 'latest' bundle", func() {
			mergeUrl := "merge-url"

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourcePacPrepKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).To(Equal("main"))
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return mergeUrl, nil
			}
			SetupPaCWebhookFunc = func(string, string, string) error {
				defer GinkgoRecover()
				Fail("Should not create webhook if GitHub application is used")
				return nil
			}

			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			annotationValue := fmt.Sprintf("{\"name\":\"%s\",\"bundle\":\"%s\"}", defaultPipelineName, "latest")
			annotations := map[string]string{defaultBuildPipelineAnnotation: annotationValue}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  annotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			pacRepo := waitPaCRepositoryCreated(resourcePacPrepKey)
			Expect(pacRepo.Spec.Params).ShouldNot(BeNil())
			existingParams := map[string]string{}
			for _, param := range *pacRepo.Spec.Params {
				existingParams[param.Name] = param.Value
			}
			val, ok := existingParams[pacCustomParamAppstudioWorkspace]
			Expect(ok).Should(BeTrue())
			Expect(val).Should(Equal("build"))

			waitPaCFinalizerOnComponent(resourcePacPrepKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "enabled", 0, "", mergeUrl)
			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
		})

		It("should fail to submit PR if build pipeline annotation isn't valid json", func() {
			annotations := map[string]string{defaultBuildPipelineAnnotation: "wrong"}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  annotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			expectError := boerrors.NewBuildOpError(boerrors.EFailedToParsePipelineAnnotation, nil)

			buildStatus := readBuildStatus(getComponent(resourcePacPrepKey))
			Expect(buildStatus).ToNot(BeNil())
			errorMessage := fmt.Sprintf("%d: %s", expectError.GetErrorId(), expectError.ShortError())
			Expect(buildStatus.Message).To(ContainSubstring(errorMessage))
		})

		It("should fail to submit PR if build pipeline annotation has non existing pipeline", func() {
			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			annotationValue := fmt.Sprintf("{\"name\":\"%s\",\"bundle\":\"%s\"}", "wrong-pipeline", "latest")
			annotations := map[string]string{defaultBuildPipelineAnnotation: annotationValue}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  annotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			expectError := boerrors.NewBuildOpError(boerrors.EBuildPipelineInvalid, nil)

			buildStatus := readBuildStatus(getComponent(resourcePacPrepKey))
			Expect(buildStatus).ToNot(BeNil())
			errorMessage := fmt.Sprintf("%d: %s", expectError.GetErrorId(), expectError.ShortError())
			Expect(buildStatus.Message).To(ContainSubstring(errorMessage))
			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
		})

		It("should fail to submit PR if build pipeline annotation is missing bundle and name", func() {
			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			annotationValue := "{\"some\":\"wrong\"}"
			annotations := map[string]string{defaultBuildPipelineAnnotation: annotationValue}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  annotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			expectError := boerrors.NewBuildOpError(boerrors.EWrongPipelineAnnotation, nil)

			buildStatus := readBuildStatus(getComponent(resourcePacPrepKey))
			Expect(buildStatus).ToNot(BeNil())
			errorMessage := fmt.Sprintf("%d: %s", expectError.GetErrorId(), expectError.ShortError())
			Expect(buildStatus.Message).To(ContainSubstring(errorMessage))
			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
		})

		It("should fail to submit PR if GitHub application is not installed into git repository", func() {
			gpf.CreateGitClient = func(gpf.GitClientConfig) (gp.GitProviderClient, error) {
				return nil, boerrors.NewBuildOpError(boerrors.EGitHubAppNotInstalled,
					fmt.Errorf("GitHub Application is not installed into the repository"))
			}

			EnsurePaCMergeRequestFunc = func(string, *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("Should not invoke merge request creation if GitHub application is not installed into the repository")
				return "url", nil
			}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			expectError := boerrors.NewBuildOpError(boerrors.EGitHubAppNotInstalled, nil)
			expectPacBuildStatus(resourcePacPrepKey, "error", expectError.GetErrorId(), expectError.ShortError(), "")
		})

		It("should fail to submit PR if unknown git provider is used", func() {
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "url", nil
			}

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				gitURL:       "https://my-git-instance.com/devfile-samples/devfile-sample-java-springboot-basic",
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			expectError := boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, nil)
			expectPacBuildStatus(resourcePacPrepKey, "error", expectError.GetErrorId(), expectError.ShortError(), "")
		})

		It("should fail to submit PR if PaC secret is invalid", func() {
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			deleteSecret(pacSecretKey)

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    "secret private key",
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			expectError := boerrors.NewBuildOpError(boerrors.EPaCSecretInvalid, nil)
			expectPacBuildStatus(resourcePacPrepKey, "error", expectError.GetErrorId(), expectError.ShortError(), "")
		})

		It("should fail to submit PR if PaC secret is missing", func() {
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			deleteSecret(pacSecretKey)
			deleteSecret(namespacePaCSecretKey)

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			expectError := boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound, nil)
			expectPacBuildStatus(resourcePacPrepKey, "error", expectError.GetErrorId(), expectError.ShortError(), "")
		})

		It("should successfully do PaC provision after error (when PaC GitHub Application was not installed)", func() {
			appNotInstalledErr := boerrors.NewBuildOpError(boerrors.EGitHubAppNotInstalled, nil)
			isCreateGithubClientInvoked := false
			gpf.CreateGitClient = func(gitClientConfig gpf.GitClientConfig) (gp.GitProviderClient, error) {
				isCreateGithubClientInvoked = true
				return nil, appNotInstalledErr
			}
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			Eventually(func() bool {
				return isCreateGithubClientInvoked
			}, timeout, interval).Should(BeTrue())

			// Ensure no more retries after permanent error
			gpf.CreateGitClient = func(gitClientConfig gpf.GitClientConfig) (gp.GitProviderClient, error) {
				defer GinkgoRecover()
				Fail("Should not retry PaC provision on permanent error")
				return nil, nil
			}
			Consistently(func() bool {
				expectPacBuildStatus(resourcePacPrepKey, "error", appNotInstalledErr.GetErrorId(), appNotInstalledErr.ShortError(), "")
				return true
			}, ensureTimeout, interval).Should(BeTrue())

			// Suppose PaC GH App is installed
			gpf.CreateGitClient = func(gitClientConfig gpf.GitClientConfig) (gp.GitProviderClient, error) {
				return testGitProviderClient, nil
			}
			mergeUrl := "merge-url"
			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return mergeUrl, nil
			}

			// Retry
			setComponentBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "enabled", 0, "", mergeUrl)
		})

		It("should not copy PaC secret into local namespace if GitHub application is used", func() {
			deleteSecret(namespacePaCSecretKey)

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCRepositoryCreated(resourcePacPrepKey)

			ensureSecretNotCreated(namespacePaCSecretKey)
		})

		It("should successfully submit PR with PaC definitions using token", func() {
			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourcePacPrepKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).ToNot(BeEmpty())
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}

			pacSecretData := map[string]string{"password": "ghp_token"}
			createSCMSecret(namespacePaCSecretKey, pacSecretData, corev1.SecretTypeBasicAuth, map[string]string{})

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			waitSecretCreated(namespacePaCSecretKey)
			waitPaCRepositoryCreated(resourcePacPrepKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
		})

		It("should provision PaC definitions after initial build, use simple build while PaC enabled, and be able to switch back to simple build only", func() {
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("Should not create PaC configuration PR when using simple build")
				return "url", nil
			}

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestTriggerSimpleBuildAnnotationValue)

			waitOneInitialPipelineRunCreated(resourcePacPrepKey)

			expectSimpleBuildStatus(resourcePacPrepKey, 0, "", false)

			// Do PaC provision
			ResetTestGitProviderClient()

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return "configure-merge-url", nil
			}
			UndoPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("Should not create undo PaC configuration PR when switching to PaC build")
				return "url", nil
			}
			SetupPaCWebhookFunc = func(repoUrl string, webhookUrl string, webhookSecret string) error {
				defer GinkgoRecover()
				Fail("Should not create webhook if GitHub application is used")
				return nil
			}

			setComponentBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			waitPaCRepositoryCreated(resourcePacPrepKey)
			waitPaCFinalizerOnComponent(resourcePacPrepKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "enabled", 0, "", "configure-merge-url")

			// Request simple build while PaC is enabled
			deleteComponentPipelineRuns(resourcePacPrepKey)

			setComponentBuildRequest(resourcePacPrepKey, BuildRequestTriggerSimpleBuildAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			waitOneInitialPipelineRunCreated(resourcePacPrepKey)

			expectSimpleBuildStatus(resourcePacPrepKey, 0, "", false)

			// Do PaC unprovision
			ResetTestGitProviderClient()

			isRemovePaCPullRequestInvoked := false
			UndoPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isRemovePaCPullRequestInvoked = true
				return "unconfigure-merge-url", nil
			}
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("Should not create PaC configuration PR when switching to simple build")
				return "url", nil
			}
			DeletePaCWebhookFunc = func(repoUrl string, webhookUrl string) error {
				defer GinkgoRecover()
				Fail("Should not delete webhook if GitHub application is used")
				return nil
			}

			// Request PaC unprovision
			setComponentBuildRequest(resourcePacPrepKey, BuildRequestUnconfigurePaCAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			waitPaCFinalizerOnComponentGone(resourcePacPrepKey)
			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "disabled", 0, "", "unconfigure-merge-url")

			// Request simple build
			deleteComponentPipelineRuns(resourcePacPrepKey)

			setComponentBuildRequest(resourcePacPrepKey, BuildRequestTriggerSimpleBuildAnnotationValue)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			waitOneInitialPipelineRunCreated(resourcePacPrepKey)

			expectSimpleBuildStatus(resourcePacPrepKey, 0, "", false)
		})

		It("should reuse the same webhook secret for multi component repository", func() {
			var webhookSecretStrings []string
			SetupPaCWebhookFunc = func(repoUrl string, webhookUrl string, webhookSecret string) error {
				webhookSecretStrings = append(webhookSecretStrings, webhookSecret)
				return nil
			}

			pacSecretData := map[string]string{"password": "ghp_token"}
			createSCMSecret(namespacePaCSecretKey, pacSecretData, corev1.SecretTypeBasicAuth, map[string]string{})

			component1Key := resourcePacPrepKey
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			deleteAllPaCRepositories(component1Key.Namespace)

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: component1Key,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourcePacPrepKey.Name,
				gitRevision:  "main",
			}, BuildRequestConfigurePaCAnnotationValue)

			defer deletePaCRepository(component2Key)
			defer deleteComponent(component2Key)

			waitComponentAnnotationGone(component1Key, BuildRequestAnnotationName)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			pacRepository := waitPaCRepositoryCreated(component1Key)
			Expect(pacRepository.OwnerReferences).To(HaveLen(2))

			waitSecretCreated(namespacePaCSecretKey)
			waitSecretCreated(webhookSecretKey)

			Expect(len(webhookSecretStrings)).To(BeNumerically(">", 0))
			for _, webhookSecret := range webhookSecretStrings {
				Expect(webhookSecret).To(Equal(webhookSecretStrings[0]))
			}
		})

		It("should use different webhook secrets for different components of the same application", func() {
			var webhookSecretStrings []string
			SetupPaCWebhookFunc = func(repoUrl string, webhookUrl string, webhookSecret string) error {
				webhookSecretStrings = append(webhookSecretStrings, webhookSecret)
				return nil
			}

			pacSecretData := map[string]string{"password": "ghp_token"}
			createSCMSecret(namespacePaCSecretKey, pacSecretData, corev1.SecretTypeBasicAuth, map[string]string{})

			component1Key := resourcePacPrepKey
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: component1Key,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey:   component2Key,
				annotations:    defaultPipelineAnnotations,
				containerImage: "registry.io/username/image2:tag2",
				gitURL:         "https://github.com/devfile-samples/devfile-sample-go-basic",
			}, BuildRequestConfigurePaCAnnotationValue)
			defer deletePaCRepository(component2Key)
			defer deleteComponent(component2Key)

			waitSecretCreated(namespacePaCSecretKey)
			waitSecretCreated(webhookSecretKey)

			waitPaCRepositoryCreated(component1Key)
			waitPaCRepositoryCreated(component2Key)

			waitComponentAnnotationGone(component1Key, BuildRequestAnnotationName)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			Expect(len(webhookSecretStrings)).To(Equal(2))
			Expect(webhookSecretStrings[0]).ToNot(Equal(webhookSecretStrings[1]))
		})

		It("should set error in status if invalid build action requested", func() {
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourcePacPrepKey,
				annotations:  defaultPipelineAnnotations,
			}, "non-existing-build-request")
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			buildStatus := readBuildStatus(getComponent(resourcePacPrepKey))
			Expect(buildStatus).ToNot(BeNil())
			Expect(buildStatus.Message).To(ContainSubstring("unexpected build request"))
		})

		It("should do nothing if a gitsource is missing from component", func() {
			EnsurePaCMergeRequestFunc = func(string, *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			component := &appstudiov1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        resourcePacPrepKey.Name,
					Namespace:   HASAppNamespace,
					Annotations: defaultPipelineAnnotations,
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  resourcePacPrepKey.Name,
					Application:    HASAppName,
					ContainerImage: "quay.io/test/image:latest",
				},
			}
			createComponentCustom(component)
			setComponentBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

			ensureNoPipelineRunsCreated(resourcePacPrepKey)
		})

		It("should set default branch as base branch when Revision is not set", func() {
			const repoDefaultBranch = "default-branch"
			GetDefaultBranchFunc = func(repoUrl string) (string, error) {
				return repoDefaultBranch, nil
			}

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Expect(d.BaseBranchName).To(Equal(repoDefaultBranch))
				for _, file := range d.Files {
					var prYaml tektonapi.PipelineRun
					if err := yaml.Unmarshal(file.Content, &prYaml); err != nil {
						return "", err
					}
					targetBranch := prYaml.Annotations[pacCelExpressionAnnotationName]
					Expect(targetBranch).To(ContainSubstring(fmt.Sprintf(`target_branch == "%s"`, repoDefaultBranch)))
				}
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}

			component := getSampleComponentData(resourcePacPrepKey)
			// Unset Revision so that GetDefaultBranch function is called to use the default branch
			// set in the remote component repository
			component.Spec.Source.GitSource.Revision = ""
			component.Annotations[BuildRequestAnnotationName] = BuildRequestConfigurePaCAnnotationValue
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue

			createComponentCustom(component)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Test Pipelines as Code build clean up", func() {
		var resourceCleanupKey = types.NamespacedName{Name: HASCompName + "-cleanup", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(BuildServiceNamespaceName)

			ResetTestGitProviderClient()
		})

		_ = AfterEach(func() {
			deleteSecret(webhookSecretKey)
			deleteSecret(namespacePaCSecretKey)

			deleteSecret(pacSecretKey)
			deleteRoute(pacRouteKey)
			deleteComponent(resourceCleanupKey)
		})

		It("should successfully unconfigure PaC even when it isn't able to get PaC webhook", func() {
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			pacSecretData = map[string]string{}
			deleteSecret(pacSecretKey)
			createSecret(pacSecretKey, pacSecretData)
			deleteRoute(pacRouteKey)

			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponentGone(resourceCleanupKey)
			waitDoneMessageOnComponent(resourceCleanupKey)
			expectPacBuildStatus(resourceCleanupKey, "disabled", 0, "", UndoPacMergeRequestURL)
		})

		It("should successfully unconfigure PaC, removal MR not needed", func() {
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)
			UndoPaCMergeRequestFunc = func(repoUrl string, data *gp.MergeRequestData) (webUrl string, err error) {
				return "", nil
			}

			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponentGone(resourceCleanupKey)
			waitDoneMessageOnComponent(resourceCleanupKey)
			expectPacBuildStatus(resourceCleanupKey, "disabled", 0, "", "")
		})

		It("should successfully submit PR with PaC definitions removal using GitHub application, remove all incomings and incoming secret (only current component is using)", func() {
			mergeUrl := "merge-url"
			isRemovePaCPullRequestInvoked := false
			UndoPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
				isRemovePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourceCleanupKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).ToNot(BeEmpty())
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return mergeUrl, nil
			}
			DeletePaCWebhookFunc = func(string, string) error {
				defer GinkgoRecover()
				Fail("Should not try to delete webhook if GitHub application is used")
				return nil
			}

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			// provision component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			repository := waitPaCRepositoryCreated(resourceCleanupKey)
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			// update repository with multiple incomings
			incoming := []pacv1alpha1.Incoming{{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}, Targets: []string{"main"}}}
			repository.Spec.Incomings = &incoming
			Expect(k8sClient.Update(ctx, repository)).Should(Succeed())

			// create incoming secret
			component := getComponent(resourceCleanupKey)
			incomingSecretData := map[string]string{
				pacIncomingSecretKey: "secret password",
			}
			incomingSecretResourceKey := types.NamespacedName{Namespace: component.Namespace, Name: incomingSecretName}
			createSecret(incomingSecretResourceKey, incomingSecretData)
			defer deleteSecret(incomingSecretResourceKey)

			// unprovision
			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)

			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourceCleanupKey, "disabled", 0, "", mergeUrl)
			waitComponentAnnotationGone(resourceCleanupKey, BuildRequestAnnotationName)
			waitSecretGone(incomingSecretResourceKey)

			repository = waitPaCRepositoryCreated(resourceCleanupKey)
			Expect(repository.Spec.Incomings).To(BeNil())
		})

		It("should successfully submit PR with PaC definitions removal, remove 1 incoming and keep secret for another component", func() {
			// tests multiple entries in incomings for components (when incomings are manually updated)
			mergeUrl := "merge-url"
			UndoPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
				return mergeUrl, nil
			}
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			// provision 1st component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)
			expectPacBuildStatus(resourceCleanupKey, "enabled", 0, "", mergeUrl)

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourceCleanupKey.Name,
				gitRevision:  "another",
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(component2Key)
			expectPacBuildStatus(component2Key, "enabled", 0, "", mergeUrl)
			defer deleteComponent(component2Key)

			component := getComponent(resourceCleanupKey)
			repository := waitPaCRepositoryCreated(resourceCleanupKey)
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			// update repository with multiple incomings
			incoming := []pacv1alpha1.Incoming{
				// 2 components using same repository and different branches
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}, Targets: []string{"main"}},
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}, Targets: []string{"another"}}}
			repository.Spec.Incomings = &incoming
			Expect(k8sClient.Update(ctx, repository)).Should(Succeed())

			// create incoming secret
			incomingSecretData := map[string]string{
				pacIncomingSecretKey: "secret password",
			}
			incomingSecretResourceKey := types.NamespacedName{Namespace: component.Namespace, Name: incomingSecretName}
			createSecret(incomingSecretResourceKey, incomingSecretData)
			defer deleteSecret(incomingSecretResourceKey)

			// unprovision 1st component
			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)
			expectPacBuildStatus(resourceCleanupKey, "disabled", 0, "", mergeUrl)
			waitComponentAnnotationGone(resourceCleanupKey, BuildRequestAnnotationName)

			// secret will still be there for another component
			waitSecretCreated(incomingSecretResourceKey)

			repository = waitPaCRepositoryCreated(resourceCleanupKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"another"}))
		})

		It("should successfully submit PR with PaC definitions removal on component deletion", func() {
			isRemovePaCPullRequestInvoked := false
			UndoPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
				isRemovePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourceCleanupKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).ToNot(BeEmpty())
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			DeletePaCWebhookFunc = func(string, string) error {
				defer GinkgoRecover()
				Fail("Should not try to delete webhook if GitHub application is used")
				return nil
			}

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			deleteComponent(resourceCleanupKey)

			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
		})

		It("should successfully submit merge request with PaC definitions removal using token", func() {
			mergeUrl := "merge-url"
			isRemovePaCPullRequestInvoked := false
			UndoPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
				isRemovePaCPullRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourceCleanupKey.Name))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.BranchName).ToNot(BeEmpty())
				Expect(d.BaseBranchName).ToNot(BeEmpty())
				Expect(d.Title).ToNot(BeEmpty())
				Expect(d.Text).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return mergeUrl, nil
			}
			isDeletePaCWebhookInvoked := false
			DeletePaCWebhookFunc = func(repoUrl string, webhookUrl string) error {
				isDeletePaCWebhookInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourceCleanupKey.Name))
				Expect(webhookUrl).To(Equal(pacWebhookUrl))
				return nil
			}

			pacSecretData := map[string]string{"password": "ghp_token"}
			createSCMSecret(namespacePaCSecretKey, pacSecretData, corev1.SecretTypeBasicAuth, map[string]string{})

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)

			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isDeletePaCWebhookInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourceCleanupKey, "disabled", 0, "", mergeUrl)
		})

		It("should not block component deletion if PaC definitions removal failed", func() {
			UndoPaCMergeRequestFunc = func(string, *gp.MergeRequestData) (webUrl string, err error) {
				return "", fmt.Errorf("failed to create PR")
			}
			DeletePaCWebhookFunc = func(string, string) error {
				return fmt.Errorf("failed to delete webhook")
			}

			pacSecretData := map[string]string{"password": "ghp_token"}
			createSCMSecret(namespacePaCSecretKey, pacSecretData, corev1.SecretTypeBasicAuth, map[string]string{})

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			// deleteComponent waits until the component is gone
			deleteComponent(resourceCleanupKey)
		})

		var assertCloseUnmergedMergeRequest = func(expectedBaseBranch string, sourceBranchExists bool) {
			UndoPaCMergeRequestFunc = func(string, *gp.MergeRequestData) (webUrl string, err error) {
				defer GinkgoRecover()
				Fail("Should close unmerged PaC configuration merge request instead of deleting .tekton/ directory")
				return "", nil
			}

			component := getSampleComponentData(resourceCleanupKey)
			if expectedBaseBranch == "" {
				expectedBaseBranch = component.Spec.Source.GitSource.Revision
			} else {
				component.Spec.Source.GitSource.Revision = ""
			}
			component.Annotations[BuildRequestAnnotationName] = BuildRequestConfigurePaCAnnotationValue
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue

			expectedSourceBranch := pacMergeRequestSourceBranchPrefix + component.Name

			isFindOnboardingMergeRequestInvoked := false
			FindUnmergedPaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (*gp.MergeRequest, error) {
				isFindOnboardingMergeRequestInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourceCleanupKey.Name))
				Expect(d.BranchName).Should(Equal(expectedSourceBranch))
				Expect(d.BaseBranchName).Should(Equal(expectedBaseBranch))
				return &gp.MergeRequest{
					WebUrl: "url",
				}, nil
			}

			isDeleteBranchInvoked := false
			DeleteBranchFunc = func(repoUrl string, branchName string) (bool, error) {
				isDeleteBranchInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resourceCleanupKey.Name))
				Expect(branchName).Should(Equal(expectedSourceBranch))
				return sourceBranchExists, nil
			}

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentCustom(component)
			waitComponentAnnotationGone(resourceCleanupKey, BuildRequestAnnotationName)
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			deleteComponent(resourceCleanupKey)

			Eventually(func() bool {
				return isFindOnboardingMergeRequestInvoked
			}, timeout, interval).Should(BeTrue(),
				"FindUnmergedPaCMergeRequest should have been invoked")
			Eventually(func() bool {
				return isDeleteBranchInvoked
			}, timeout, interval).Should(BeTrue(),
				"DeleteBranch should have been invoked")
		}

		It("should close unmerged PaC pull request opened based on branch specified in Revision", func() {
			assertCloseUnmergedMergeRequest("", true)
		})

		It("should not error when attempt of close unmerged PaC pull request by deleting a non-existing source branch", func() {
			assertCloseUnmergedMergeRequest("", false)
		})

		It("should close unmerged PaC pull request opened based on default branch", func() {
			defaultBranch := "devel"
			GetDefaultBranchFunc = func(repoUrl string) (string, error) {
				return defaultBranch, nil
			}
			assertCloseUnmergedMergeRequest(defaultBranch, true)
		})

		It("should fail to unconfigure PaC when pac secret is missing", func() {
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)
			deleteSecret(pacSecretKey)

			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponentGone(resourceCleanupKey)
			waitDoneMessageOnComponent(resourceCleanupKey)

			expectError := boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound, nil)
			expectPacBuildStatus(resourceCleanupKey, "error", expectError.GetErrorId(), expectError.ShortError(), "")
		})

		It("should fail to unconfigure PaC if it isn't able to detect git provider", func() {
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourceCleanupKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			component := getComponent(resourceCleanupKey)
			component.Spec.Source.GitSource.URL = "wrong"
			Expect(k8sClient.Update(ctx, component)).To(Succeed())
			waitPaCFinalizerOnComponent(resourceCleanupKey)

			setComponentBuildRequest(resourceCleanupKey, BuildRequestUnconfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponentGone(resourceCleanupKey)
			waitDoneMessageOnComponent(resourceCleanupKey)

			expectError := boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, nil)
			expectPacBuildStatus(resourceCleanupKey, "error", expectError.GetErrorId(), expectError.ShortError(), "")
		})
	})

	Context("Test Pipelines as Code multi component git repository", func() {
		const (
			multiComponentGitRepositoryUrl = "https://github.com/samples/multi-component-repository"
		)
		var anotherComponentKey = types.NamespacedName{Name: HASCompName + "-multicomp", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			deleteAllPaCRepositories(anotherComponentKey.Namespace)

			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(BuildServiceNamespaceName)
			ResetTestGitProviderClient()

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: anotherComponentKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitComponentAnnotationGone(anotherComponentKey, BuildRequestAnnotationName)
			waitPaCRepositoryCreated(anotherComponentKey)
		})

		_ = AfterEach(func() {
			ResetTestGitProviderClient()
			deleteComponentPipelineRuns(anotherComponentKey)
			deleteComponent(anotherComponentKey)
			deleteSecret(pacSecretKey)
			deleteRoute(pacRouteKey)
			deletePaCRepository(anotherComponentKey)
		})

		It("should reuse existing PaC repository for multi component git repository", func() {
			component1Key := types.NamespacedName{Name: "test-multi-component1", Namespace: HASAppNamespace}
			component2Key := types.NamespacedName{Name: "test-multi-component2", Namespace: HASAppNamespace}

			pacRepositoriesList := &pacv1alpha1.RepositoryList{}
			pacRepository := &pacv1alpha1.Repository{}
			err := k8sClient.Get(ctx, component1Key, pacRepository)
			Expect(k8sErrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, component2Key, pacRepository)
			Expect(k8sErrors.IsNotFound(err)).To(BeTrue())

			component1PaCMergeRequestCreated := false
			component2PaCMergeRequestCreated := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				if strings.Contains(d.Files[0].FullPath, component1Key.Name) {
					component1PaCMergeRequestCreated = true
					return "url1", nil
				} else if strings.Contains(d.Files[0].FullPath, component2Key.Name) {
					component2PaCMergeRequestCreated = true
					return "url2", nil
				} else {
					Fail("Unknown component in EnsurePaCMergeRequest")
				}
				return "", nil
			}

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: component1Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       multiComponentGitRepositoryUrl,
			}, BuildRequestConfigurePaCAnnotationValue)
			defer deleteComponent(component1Key)
			waitComponentAnnotationGone(component1Key, BuildRequestAnnotationName)
			Eventually(func() bool { return component1PaCMergeRequestCreated }, timeout, interval).Should(BeTrue())
			waitPaCRepositoryCreated(component1Key)
			defer deletePaCRepository(component1Key)
			Expect(k8sClient.Get(ctx, component1Key, pacRepository)).To(Succeed())
			Expect(pacRepository.OwnerReferences).To(HaveLen(1))
			Expect(pacRepository.OwnerReferences[0].Name).To(Equal(component1Key.Name))
			Expect(pacRepository.OwnerReferences[0].Kind).To(Equal("Component"))
			err = k8sClient.Get(ctx, component2Key, pacRepository)
			Expect(k8sErrors.IsNotFound(err)).To(BeTrue())
			Expect(k8sClient.List(ctx, pacRepositoriesList, &client.ListOptions{Namespace: component1Key.Namespace})).To(Succeed())
			Expect(pacRepositoriesList.Items).To(HaveLen(2)) // 2-nd repository for the anotherComponentKey component

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       multiComponentGitRepositoryUrl,
			}, BuildRequestConfigurePaCAnnotationValue)
			defer deleteComponent(component2Key)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)
			Eventually(func() bool { return component2PaCMergeRequestCreated }, timeout, interval).Should(BeTrue())
			Expect(k8sClient.Get(ctx, component1Key, pacRepository)).To(Succeed())
			Expect(pacRepository.OwnerReferences).To(HaveLen(2))
			Expect(pacRepository.OwnerReferences[0].Name).To(Equal(component1Key.Name))
			Expect(pacRepository.OwnerReferences[0].Kind).To(Equal("Component"))
			Expect(pacRepository.OwnerReferences[1].Name).To(Equal(component2Key.Name))
			Expect(pacRepository.OwnerReferences[1].Kind).To(Equal("Component"))
			err = k8sClient.Get(ctx, component2Key, pacRepository)
			Expect(k8sErrors.IsNotFound(err)).To(BeTrue())
			Expect(k8sClient.List(ctx, pacRepositoriesList, &client.ListOptions{Namespace: component1Key.Namespace})).To(Succeed())
			Expect(pacRepositoriesList.Items).To(HaveLen(2)) // 2-nd repository for the anotherComponentKey component
		})
	})

	Context("Test simple build flow", func() {
		var resouceSimpleBuildKey = types.NamespacedName{Name: HASCompName + "-simpleannotation", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(BuildServiceNamespaceName)
			ResetTestGitProviderClient()

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
		})

		_ = AfterEach(func() {
			deleteSecret(pacSecretKey)
			deleteComponentPipelineRuns(resouceSimpleBuildKey)
			deleteComponent(resouceSimpleBuildKey)
			// wait for pruner operator to finish, so it won't prune runs from new test
			time.Sleep(time.Second)
		})

		It("should submit initial build on component creation", func() {
			gitSourceSHA := "d1a9e858489d1515621398fb02942da068f1c956"
			isGetBranchShaInvoked := false
			GetBranchShaFunc = func(repoUrl string, branchName string) (string, error) {
				isGetBranchShaInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				return gitSourceSHA, nil
			}
			GetBrowseRepositoryAtShaLinkFunc = func(repoUrl, sha string) string {
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				Expect(sha).To(Equal(gitSourceSHA))
				return "https://github.com/devfile-samples/devfile-sample-java-springboot-basic?rev=" + gitSourceSHA
			}

			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})

			Eventually(func() bool {
				return isGetBranchShaInvoked
			}, timeout, interval).Should(BeTrue())

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)

			// Check pipeline run labels and annotations
			pipelineRun := listComponentPipelineRuns(resouceSimpleBuildKey)[0]
			Expect(pipelineRun.Annotations[gitCommitShaAnnotationName]).To(Equal(gitSourceSHA))
			Expect(pipelineRun.Annotations[gitRepoAtShaAnnotationName]).To(
				Equal("https://github.com/devfile-samples/devfile-sample-java-springboot-basic?rev=" + gitSourceSHA))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal(defaultPipelineName))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))
		})

		It("should submit initial build on component creation, using 'latest' bundle", func() {
			gitSourceSHA := "d1a9e858489d1515621398fb02942da068f1c956"
			isGetBranchShaInvoked := false
			GetBranchShaFunc = func(repoUrl string, branchName string) (string, error) {
				isGetBranchShaInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				return gitSourceSHA, nil
			}
			GetBrowseRepositoryAtShaLinkFunc = func(repoUrl, sha string) string {
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				Expect(sha).To(Equal(gitSourceSHA))
				return "https://github.com/devfile-samples/devfile-sample-java-springboot-basic?rev=" + gitSourceSHA
			}

			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			annotationValue := fmt.Sprintf("{\"name\":\"%s\",\"bundle\":\"%s\"}", defaultPipelineName, "latest")
			annotations := map[string]string{defaultBuildPipelineAnnotation: annotationValue}
			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  annotations})

			Eventually(func() bool {
				return isGetBranchShaInvoked
			}, timeout, interval).Should(BeTrue())

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)

			// Check pipeline run labels and annotations
			pipelineRun := listComponentPipelineRuns(resouceSimpleBuildKey)[0]
			Expect(pipelineRun.Annotations[gitCommitShaAnnotationName]).To(Equal(gitSourceSHA))
			Expect(pipelineRun.Annotations[gitRepoAtShaAnnotationName]).To(
				Equal("https://github.com/devfile-samples/devfile-sample-java-springboot-basic?rev=" + gitSourceSHA))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal(defaultPipelineName))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
		})

		It("should submit initial build on component creation, even when pipeline annotation is missing, will add default one", func() {
			gitSourceSHA := "d1a9e858489d1515621398fb02942da068f1c956"
			isGetBranchShaInvoked := false
			GetBranchShaFunc = func(repoUrl string, branchName string) (string, error) {
				isGetBranchShaInvoked = true
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				return gitSourceSHA, nil
			}
			GetBrowseRepositoryAtShaLinkFunc = func(repoUrl, sha string) string {
				Expect(repoUrl).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				Expect(sha).To(Equal(gitSourceSHA))
				return "https://github.com/devfile-samples/devfile-sample-java-springboot-basic?rev=" + gitSourceSHA
			}

			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			annotations := map[string]string{}
			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  annotations})

			Eventually(func() bool {
				return isGetBranchShaInvoked
			}, timeout, interval).Should(BeTrue())

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)

			// Check pipeline run labels and annotations
			pipelineRun := listComponentPipelineRuns(resouceSimpleBuildKey)[0]
			Expect(pipelineRun.Annotations[gitCommitShaAnnotationName]).To(Equal(gitSourceSHA))
			Expect(pipelineRun.Annotations[gitRepoAtShaAnnotationName]).To(
				Equal("https://github.com/devfile-samples/devfile-sample-java-springboot-basic?rev=" + gitSourceSHA))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal(defaultPipelineName))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
		})

		It("should submit initial build on component creation with sha revision", func() {
			gitSourceSHA := "d1a9e858489d1515621398fb02942da068f1c956"
			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.Source.GitSource.Revision = gitSourceSHA
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)

			// Check pipeline run labels and annotations
			pipelineRun := listComponentPipelineRuns(resouceSimpleBuildKey)[0]
			Expect(pipelineRun.Annotations[gitCommitShaAnnotationName]).To(Equal(gitSourceSHA))
			Expect(pipelineRun.Annotations[gitRepoAtShaAnnotationName]).To(Equal(DefaultBrowseRepository + gitSourceSHA))
		})

		It("should be able to retrigger simple build", func() {
			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			deleteComponentPipelineRuns(resouceSimpleBuildKey)

			setComponentBuildRequest(resouceSimpleBuildKey, BuildRequestTriggerSimpleBuildAnnotationValue)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)
		})

		It("should run simple build and create pipeline service account when it doesn't exist", func() {
			serviceAccountName := types.NamespacedName{Name: buildPipelineServiceAccountName, Namespace: "default"}
			serviceAccount := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, serviceAccountName, serviceAccount)).To(Succeed())

			// remove pipeline service account so it can be re-created
			Expect(k8sClient.Delete(ctx, serviceAccount)).Should(Succeed())
			Eventually(func() bool {
				return k8sErrors.IsNotFound(k8sClient.Get(ctx, serviceAccountName, serviceAccount))
			}, timeout, interval).Should(BeTrue())

			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			waitDoneMessageOnComponent(resouceSimpleBuildKey)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)
		})

		It("should submit initial build if retrieving of git commit SHA failed", func() {
			isGetBranchShaInvoked := false
			GetBranchShaFunc = func(repoUrl string, branchName string) (string, error) {
				isGetBranchShaInvoked = true
				return "", fmt.Errorf("failed to get git commit SHA")
			}

			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})

			Eventually(func() bool {
				return isGetBranchShaInvoked
			}, timeout, interval).Should(BeTrue())

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)
		})

		It("should submit initial build for private git repository", func() {
			gitSecretName := "git-secret"

			isRepositoryPublicInvoked := false
			IsRepositoryPublicFunc = func(repoUrl string) (bool, error) {
				isRepositoryPublicInvoked = true
				return false, nil
			}
			isGetBranchShaInvoked := false
			GetBranchShaFunc = func(repoUrl string, branchName string) (string, error) {
				isGetBranchShaInvoked = true
				return "", fmt.Errorf("failed to get git commit SHA")
			}

			gitSecretKey := types.NamespacedName{Name: gitSecretName, Namespace: resouceSimpleBuildKey.Namespace}
			createSecret(gitSecretKey, map[string]string{})
			defer deleteSecret(gitSecretKey)

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.Secret = gitSecretName
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)

			Eventually(func() bool { return isRepositoryPublicInvoked }, timeout, interval).Should(BeTrue())
			Eventually(func() bool { return isGetBranchShaInvoked }, timeout, interval).Should(BeTrue())

			// Wait until all resources created
			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)

			// Check the pipeline run and its resources
			pipelineRuns := listComponentPipelineRuns(resouceSimpleBuildKey)
			Expect(len(pipelineRuns)).To(Equal(1))
			pipelineRun := pipelineRuns[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(resouceSimpleBuildKey.Name))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal(defaultPipelineName))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(getPipelineName(pipelineRun.Spec.PipelineRef)).To(Equal(defaultPipelineName))
			Expect(getPipelineBundle(pipelineRun.Spec.PipelineRef)).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.Params).ToNot(BeEmpty())
			for _, p := range pipelineRun.Spec.Params {
				switch p.Name {
				case "output-image":
					Expect(p.Value.StringVal).ToNot(BeEmpty())
					Expect(strings.HasPrefix(p.Value.StringVal, "docker.io/foo/customized:"+resouceSimpleBuildKey.Name+"-build-"))
				case "git-url":
					Expect(p.Value.StringVal).To(Equal(SampleRepoLink + "-" + resouceSimpleBuildKey.Name))
				case "revision":
					Expect(p.Value.StringVal).To(Equal("main"))
				}
			}

			Expect(pipelineRun.Spec.Workspaces).To(Not(BeEmpty()))
			isWorkspaceWorkspaceExist := false
			isWorkspaceGitAuthExist := false
			for _, w := range pipelineRun.Spec.Workspaces {
				if w.Name == "workspace" {
					isWorkspaceWorkspaceExist = true
					Expect(w.VolumeClaimTemplate).NotTo(
						Equal(nil), "PipelineRun should have its own volumeClaimTemplate.")
				}
				if w.Name == "git-auth" {
					isWorkspaceGitAuthExist = true
					Expect(w.Secret.SecretName).To(Equal(gitSecretName))
				}
			}
			Expect(isWorkspaceWorkspaceExist).To(BeTrue())
			Expect(isWorkspaceGitAuthExist).To(BeTrue())

			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)
		})

		It("should submit initial simple build, even when containerimage is missing, but is found in image repo annotation", func() {
			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.ContainerImage = ""
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue

			type RepositoryInfo struct {
				Image  string `json:"image"`
				Secret string `json:"secret"`
			}
			repoInfo := RepositoryInfo{Image: ComponentContainerImage, Secret: ""}
			repoInfoBytes, _ := json.Marshal(repoInfo)
			component.Annotations[ImageRepoAnnotationName] = string(repoInfoBytes)

			createComponentCustom(component)
			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			waitDoneMessageOnComponent(resouceSimpleBuildKey)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)
		})

		It("should fail to submit initial build for private git repository if git secret is not given", func() {
			isRepositoryPublicInvoked := false
			IsRepositoryPublicFunc = func(repoUrl string) (bool, error) {
				isRepositoryPublicInvoked = true
				return false, nil
			}
			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})

			Eventually(func() bool { return isRepositoryPublicInvoked }, timeout, interval).Should(BeTrue())

			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EComponentGitSecretNotSpecified, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should fail to submit initial build for private git repository if git secret is missing", func() {
			isRepositoryPublicInvoked := false
			IsRepositoryPublicFunc = func(repoUrl string) (bool, error) {
				isRepositoryPublicInvoked = true
				return false, nil
			}

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.Secret = "git-secret"
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)

			Eventually(func() bool { return isRepositoryPublicInvoked }, timeout, interval).Should(BeTrue())

			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EComponentGitSecretMissing, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should not submit initial build if pac secret is missing", func() {
			deleteSecret(pacSecretKey)
			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})
			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should do nothing if simple build already happened (and PaC is not used)", func() {
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Annotations = make(map[string]string)
			component.Annotations[BuildStatusAnnotationName] = "{simple:{\"build-start-time\": \"time\"}}"
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build if a gitsource is missing from component", func() {
			component := &appstudiov1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        resouceSimpleBuildKey.Name,
					Namespace:   HASAppNamespace,
					Annotations: defaultPipelineAnnotations,
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  resouceSimpleBuildKey.Name,
					Application:    HASAppName,
					ContainerImage: "quay.io/test/image:latest",
				},
			}
			createComponentCustom(component)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial simple build if git provider can't be detected)", func() {
			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.Source.GitSource.URL = "wrong"
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)
			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should not submit initial simple build if public repository check fails", func() {
			isRepositoryPublicInvoked := false
			IsRepositoryPublicFunc = func(repoUrl string) (bool, error) {
				isRepositoryPublicInvoked = true
				return false, fmt.Errorf("failed to check repository")
			}

			createCustomComponentWithoutBuildRequest(componentConfig{
				componentKey: resouceSimpleBuildKey,
				annotations:  defaultPipelineAnnotations})
			Eventually(func() bool { return isRepositoryPublicInvoked }, timeout, interval).Should(BeTrue())

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build if ContainerImage is not set)", func() {
			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.ContainerImage = ""
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build when request is empty)", func() {
			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Annotations[BuildRequestAnnotationName] = ""
			component.Annotations[defaultBuildPipelineAnnotation] = defaultPipelineAnnotationValue
			createComponentCustom(component)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})
	})

	Context("Test Pipelines as Code trigger build", func() {
		var resourcePacTriggerKey = types.NamespacedName{Name: HASCompName + "-pactrigger", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(BuildServiceNamespaceName)
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			ResetTestGitProviderClient()
		})

		_ = AfterEach(func() {
			deleteComponent(resourcePacTriggerKey)
			deletePaCRepository(resourcePacTriggerKey)

			deleteSecret(webhookSecretKey)
			deleteSecret(namespacePaCSecretKey)

			deleteSecret(pacSecretKey)
			deleteRoute(pacRouteKey)
		})

		It("should successfully trigger PaC build", func() {
			mergeUrl := "merge-url"

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacTriggerKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)

			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)
			component := getComponent(resourcePacTriggerKey)

			pacWebhookRoute := &routev1.Route{}
			Expect(k8sClient.Get(ctx, pacRouteKey, pacWebhookRoute)).To(Succeed())
			webhookTargetUrl := "https://" + pacWebhookRoute.Spec.Host
			pipelineRunName := component.Name + pipelineRunOnPushSuffix

			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			gock.InterceptClient(client)

			getHttpClientMocked := func() *http.Client {
				return client
			}
			GetHttpClientFunction = getHttpClientMocked

			defer gock.Off()

			setComponentBuildRequest(resourcePacTriggerKey, BuildRequestTriggerPaCBuildAnnotationValue)

			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)
			incomingSecretResourceKey := types.NamespacedName{Namespace: component.Namespace, Name: incomingSecretName}
			waitSecretCreated(incomingSecretResourceKey)
			defer deleteSecret(incomingSecretResourceKey)

			incomingSecret := corev1.Secret{}
			Expect(k8sClient.Get(ctx, incomingSecretResourceKey, &incomingSecret)).To(Succeed())
			secretValue := string(incomingSecret.Data[pacIncomingSecretKey][:])

			req := gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("secret", secretValue).
				MatchParam("repository", component.Name).
				MatchParam("branch", "main").
				MatchParam("pipelinerun", pipelineRunName)
			req.Reply(202).JSON(map[string]string{})

			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)

			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main"}))
		})

		It("should successfully trigger builds for 2 components with different branches in the same repo", func() {
			mergeUrl := "merge-url"

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			// provision 1st component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacTriggerKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)
			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourcePacTriggerKey.Name,
				gitRevision:  "another",
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(component2Key)
			expectPacBuildStatus(component2Key, "enabled", 0, "", mergeUrl)
			defer deleteComponent(component2Key)

			pacWebhookRoute := &routev1.Route{}
			Expect(k8sClient.Get(ctx, pacRouteKey, pacWebhookRoute)).To(Succeed())
			webhookTargetUrl := "https://" + pacWebhookRoute.Spec.Host
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			component1 := getComponent(resourcePacTriggerKey)
			pipelineRunName1 := component1.Name + pipelineRunOnPushSuffix

			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			gock.InterceptClient(client)

			getHttpClientMocked := func() *http.Client {
				return client
			}
			GetHttpClientFunction = getHttpClientMocked

			defer gock.Off()

			req := gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", component1.Name).
				MatchParam("branch", "main").
				MatchParam("pipelinerun", pipelineRunName1)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 1st component
			setComponentBuildRequest(resourcePacTriggerKey, BuildRequestTriggerPaCBuildAnnotationValue)

			incomingSecretResourceKey := types.NamespacedName{Namespace: component1.Namespace, Name: incomingSecretName}
			waitSecretCreated(incomingSecretResourceKey)
			defer deleteSecret(incomingSecretResourceKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main"}))

			component2 := getComponent(component2Key)
			pipelineRunName2 := component2.Name + pipelineRunOnPushSuffix

			req = gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", component1.Name).
				MatchParam("branch", "another").
				MatchParam("pipelinerun", pipelineRunName2)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 2nd component
			setComponentBuildRequest(component2Key, BuildRequestTriggerPaCBuildAnnotationValue)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main", "another"}))
		})

		It("should successfully trigger builds for 2 components with the same branches in the same repo", func() {
			mergeUrl := "merge-url"

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			// provision 1st component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacTriggerKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)
			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourcePacTriggerKey.Name,
				gitRevision:  "main",
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(component2Key)
			expectPacBuildStatus(component2Key, "enabled", 0, "", mergeUrl)
			defer deleteComponent(component2Key)

			pacWebhookRoute := &routev1.Route{}
			Expect(k8sClient.Get(ctx, pacRouteKey, pacWebhookRoute)).To(Succeed())
			webhookTargetUrl := "https://" + pacWebhookRoute.Spec.Host
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			component1 := getComponent(resourcePacTriggerKey)
			pipelineRunName1 := component1.Name + pipelineRunOnPushSuffix

			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			gock.InterceptClient(client)

			getHttpClientMocked := func() *http.Client {
				return client
			}
			GetHttpClientFunction = getHttpClientMocked

			defer gock.Off()

			req := gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", component1.Name).
				MatchParam("branch", "main").
				MatchParam("pipelinerun", pipelineRunName1)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 1st component
			setComponentBuildRequest(resourcePacTriggerKey, BuildRequestTriggerPaCBuildAnnotationValue)

			incomingSecretResourceKey := types.NamespacedName{Namespace: component1.Namespace, Name: incomingSecretName}
			waitSecretCreated(incomingSecretResourceKey)
			defer deleteSecret(incomingSecretResourceKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main"}))

			component2 := getComponent(component2Key)
			pipelineRunName2 := component2.Name + pipelineRunOnPushSuffix

			req = gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", component1.Name).
				MatchParam("branch", "main").
				MatchParam("pipelinerun", pipelineRunName2)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 2nd component
			setComponentBuildRequest(component2Key, BuildRequestTriggerPaCBuildAnnotationValue)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main"}))

		})

		It("should successfully trigger PaC build, multiple incomings exist, even with wrong secret", func() {
			mergeUrl := "merge-url"

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			// provision 1st component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacTriggerKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)

			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			// update repository with multiple incomings, even with wrong secret
			incoming := []pacv1alpha1.Incoming{
				// correct secret name for main branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}, Targets: []string{"main"}},
				// missing secret for first branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{}, Targets: []string{"first"}},
				// wrong secret for second branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: "wrong_name", Key: pacIncomingSecretKey}, Targets: []string{"second"}}}
			repository.Spec.Incomings = &incoming
			Expect(k8sClient.Update(ctx, repository)).Should(Succeed())

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourcePacTriggerKey.Name,
				gitRevision:  "another",
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(component2Key)
			expectPacBuildStatus(component2Key, "enabled", 0, "", mergeUrl)
			defer deleteComponent(component2Key)

			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			gock.InterceptClient(client)

			getHttpClientMocked := func() *http.Client {
				return client
			}
			GetHttpClientFunction = getHttpClientMocked

			defer gock.Off()

			pacWebhookRoute := &routev1.Route{}
			Expect(k8sClient.Get(ctx, pacRouteKey, pacWebhookRoute)).To(Succeed())
			webhookTargetUrl := "https://" + pacWebhookRoute.Spec.Host

			component2 := getComponent(component2Key)
			pipelineRunName2 := component2.Name + pipelineRunOnPushSuffix

			req := gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", repository.Name).
				MatchParam("branch", "another").
				MatchParam("pipelinerun", pipelineRunName2)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 2nd component
			setComponentBuildRequest(component2Key, BuildRequestTriggerPaCBuildAnnotationValue)
			incomingSecretResourceKey := types.NamespacedName{Namespace: component2.Namespace, Name: incomingSecretName}
			waitSecretCreated(incomingSecretResourceKey)
			defer deleteSecret(incomingSecretResourceKey)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main", "first", "second", "another"}))
		})

		It("should successfully trigger PaC build, multiple incomings exist, even with wrong secret, branch found but without secret", func() {
			mergeUrl := "merge-url"

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			// provision 1st component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacTriggerKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)

			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			// update repository with multiple incomings, even with wrong secret
			incoming := []pacv1alpha1.Incoming{
				// missing secret for main branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{}, Targets: []string{"main"}},
				// missing secret for another branch (used for 2nd component)
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{}, Targets: []string{"another"}},
				// missing secret for first branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{}, Targets: []string{"first"}},
				// wrong secret for second branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: "wrong_name", Key: pacIncomingSecretKey}, Targets: []string{"second"}}}
			repository.Spec.Incomings = &incoming
			Expect(k8sClient.Update(ctx, repository)).Should(Succeed())

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourcePacTriggerKey.Name,
				gitRevision:  "another",
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(component2Key)
			expectPacBuildStatus(component2Key, "enabled", 0, "", mergeUrl)
			defer deleteComponent(component2Key)

			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			gock.InterceptClient(client)

			getHttpClientMocked := func() *http.Client {
				return client
			}
			GetHttpClientFunction = getHttpClientMocked

			defer gock.Off()

			pacWebhookRoute := &routev1.Route{}
			Expect(k8sClient.Get(ctx, pacRouteKey, pacWebhookRoute)).To(Succeed())
			webhookTargetUrl := "https://" + pacWebhookRoute.Spec.Host

			component2 := getComponent(component2Key)
			pipelineRunName2 := component2.Name + pipelineRunOnPushSuffix

			req := gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", repository.Name).
				MatchParam("branch", "another").
				MatchParam("pipelinerun", pipelineRunName2)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 2nd component
			setComponentBuildRequest(component2Key, BuildRequestTriggerPaCBuildAnnotationValue)
			incomingSecretResourceKey := types.NamespacedName{Namespace: component2.Namespace, Name: incomingSecretName}
			waitSecretCreated(incomingSecretResourceKey)
			defer deleteSecret(incomingSecretResourceKey)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main", "another", "first", "second"}))
		})

		It("should successfully trigger PaC build, one incomings exist, but without secret", func() {
			mergeUrl := "merge-url"

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				return mergeUrl, nil
			}

			// provision 1st component
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: resourcePacTriggerKey,
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)

			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)
			incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)

			// update repository with multiple incomings, even with wrong secret
			incoming := []pacv1alpha1.Incoming{
				// missing secret for main branch
				{Type: "webhook-url", Secret: pacv1alpha1.Secret{}, Targets: []string{"main"}}}
			repository.Spec.Incomings = &incoming
			Expect(k8sClient.Update(ctx, repository)).Should(Succeed())

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentAndProcessBuildRequest(componentConfig{
				componentKey: component2Key,
				annotations:  defaultPipelineAnnotations,
				gitURL:       SampleRepoLink + "-" + resourcePacTriggerKey.Name,
				gitRevision:  "another",
			}, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(component2Key)
			expectPacBuildStatus(component2Key, "enabled", 0, "", mergeUrl)
			defer deleteComponent(component2Key)

			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
			gock.InterceptClient(client)

			getHttpClientMocked := func() *http.Client {
				return client
			}
			GetHttpClientFunction = getHttpClientMocked

			defer gock.Off()

			pacWebhookRoute := &routev1.Route{}
			Expect(k8sClient.Get(ctx, pacRouteKey, pacWebhookRoute)).To(Succeed())
			webhookTargetUrl := "https://" + pacWebhookRoute.Spec.Host

			component2 := getComponent(component2Key)
			pipelineRunName2 := component2.Name + pipelineRunOnPushSuffix

			req := gock.New(webhookTargetUrl).
				BodyString("").
				Post("/incoming").
				MatchParam("repository", repository.Name).
				MatchParam("branch", "another").
				MatchParam("pipelinerun", pipelineRunName2)
			req.Reply(202).JSON(map[string]string{})

			// trigger push pipeline for 2nd component
			setComponentBuildRequest(component2Key, BuildRequestTriggerPaCBuildAnnotationValue)
			incomingSecretResourceKey := types.NamespacedName{Namespace: component2.Namespace, Name: incomingSecretName}
			waitSecretCreated(incomingSecretResourceKey)
			defer deleteSecret(incomingSecretResourceKey)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)

			repository = waitPaCRepositoryCreated(resourcePacTriggerKey)
			Expect(repository.Spec.Incomings).ToNot(BeNil())
			Expect(len(*repository.Spec.Incomings)).To(Equal(1))
			Expect((*repository.Spec.Incomings)[0].Secret.Name).To(Equal(incomingSecretName))
			Expect((*repository.Spec.Incomings)[0].Secret.Key).To(Equal(pacIncomingSecretKey))
			Expect((*repository.Spec.Incomings)[0].Targets).To(Equal([]string{"main", "another"}))
		})
	})
})
