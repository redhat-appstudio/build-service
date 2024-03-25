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

	"github.com/devfile/api/v2/pkg/apis/workspaces/v1alpha2"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	devfile "github.com/redhat-appstudio/application-service/cdq-analysis/pkg"
	"github.com/redhat-appstudio/application-service/gitops"
	gitopsprepare "github.com/redhat-appstudio/application-service/gitops/prepare"
	buildappstudiov1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	"github.com/redhat-appstudio/build-service/pkg/git/github"
	gp "github.com/redhat-appstudio/build-service/pkg/git/gitprovider"
	gpf "github.com/redhat-appstudio/build-service/pkg/git/gitproviderfactory"
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

var _ = Describe("Component initial build controller", func() {

	const (
		pacHost       = "pac-host"
		pacWebhookUrl = "https://" + pacHost
	)

	var (
		// All related to the component resources have the same key (but different type)
		pacRouteKey           = types.NamespacedName{Name: pipelinesAsCodeRouteName, Namespace: pipelinesAsCodeNamespace}
		pacSecretKey          = types.NamespacedName{Name: gitopsprepare.PipelinesAsCodeSecretName, Namespace: buildServiceNamespaceName}
		namespacePaCSecretKey = types.NamespacedName{Name: gitopsprepare.PipelinesAsCodeSecretName, Namespace: HASAppNamespace}
		webhookSecretKey      = types.NamespacedName{Name: gitops.PipelinesAsCodeWebhooksSecretName, Namespace: HASAppNamespace}
	)

	BeforeEach(func() {
		createNamespace(buildServiceNamespaceName)
		createDefaultBuildPipelineRunSelector(defaultSelectorKey)
		github.GetAllAppInstallations = func(githubAppIdStr string, appPrivateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
			return nil, "slug", nil
		}
	})

	AfterEach(func() {
		deleteBuildPipelineRunSelector(defaultSelectorKey)
	})

	Context("Test Pipelines as Code build preparation", func() {
		var resourcePacPrepKey = types.NamespacedName{Name: HASCompName + "-pacprep", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(buildServiceNamespaceName)
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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

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

		It("should submit PR with PaC definitions converted to Tekton v1 from a v1beta1 Pipeline", func() {
			deleteBuildPipelineRunSelector(defaultSelectorKey)
			createBuildPipelineRunSelector(defaultSelectorKey, v1beta1PipelineBundle, defaultPipelineName)

			mergeUrl := "merge-url"

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				defer GinkgoRecover()
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					firstLine, _, _ := strings.Cut(string(file.Content), "\n")
					Expect(firstLine).Should(MatchRegexp("^apiVersion: tekton.dev/v1$"))
				}
				return mergeUrl, nil
			}

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

			waitPaCRepositoryCreated(resourcePacPrepKey)
			waitPaCFinalizerOnComponent(resourcePacPrepKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			expectPacBuildStatus(resourcePacPrepKey, "enabled", 0, "", mergeUrl)
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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)
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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

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

			createComponentAndProcessBuildRequest(resourcePacPrepKey, BuildRequestTriggerSimpleBuildAnnotationValue)

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

			createComponentWithBuildRequest(component1Key, BuildRequestConfigurePaCAnnotationValue)
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourcePacPrepKey.Name, "main")
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

			createComponentWithBuildRequest(component1Key, BuildRequestConfigurePaCAnnotationValue)
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey:   component2Key,
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
			}, "non-existing-build-request")
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			buildStatus := readBuildStatus(getComponent(resourcePacPrepKey))
			Expect(buildStatus).ToNot(BeNil())
			Expect(buildStatus.Message).To(ContainSubstring("unexpected build request"))
		})

		It("should do nothing if the component devfile model is not set", func() {
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			createComponent(resourcePacPrepKey)
			setComponentBuildRequest(resourcePacPrepKey, BuildRequestConfigurePaCAnnotationValue)

			ensureComponentAnnotationValue(resourcePacPrepKey, BuildRequestAnnotationName, BuildRequestConfigurePaCAnnotationValue)
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
					Name:      resourcePacPrepKey.Name,
					Namespace: HASAppNamespace,
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
			createComponentCustom(component)
			setComponentDevfileModel(resourcePacPrepKey)
			waitComponentAnnotationGone(resourcePacPrepKey, BuildRequestAnnotationName)

			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
		})

		assertLinkSecretToSa := func(secretNotEmpty bool) {
			userImageRepo := "docker.io/user/image"
			generatedImageRepo := "quay.io/appstudio/generated-image"
			generatedImageRepoSecretName := "generated-image-repo-secret"
			generatedImageRepoSecretKey := types.NamespacedName{Namespace: resourcePacPrepKey.Namespace, Name: generatedImageRepoSecretName}
			pipelineSAKey := types.NamespacedName{Namespace: resourcePacPrepKey.Namespace, Name: buildPipelineServiceAccountName}

			checkPROutputImage := func(fileContent []byte, expectedImageRepo string) {
				var prYaml tektonapi.PipelineRun
				Expect(yaml.Unmarshal(fileContent, &prYaml)).To(Succeed())
				outoutImage := ""
				for _, param := range prYaml.Spec.Params {
					if param.Name == "output-image" {
						outoutImage = param.Value.StringVal
						break
					}
				}
				Expect(outoutImage).ToNot(BeEmpty())
				Expect(outoutImage).To(ContainSubstring(expectedImageRepo))
			}

			isCreatePaCPullRequestInvoked := false
			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				checkPROutputImage(d.Files[0].Content, userImageRepo)
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}

			// Create a component with user's ContainerImage
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey:   resourcePacPrepKey,
				containerImage: userImageRepo,
			}, BuildRequestConfigurePaCAnnotationValue)

			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())

			// Switch to generated image repository
			createSecret(generatedImageRepoSecretKey, nil)
			defer deleteSecret(generatedImageRepoSecretKey)

			component := getComponent(resourcePacPrepKey)
			component.Annotations[ImageRepoGenerateAnnotationName] = "false"

			if secretNotEmpty {
				component.Annotations[ImageRepoAnnotationName] =
					fmt.Sprintf("{\"image\":\"%s\",\"secret\":\"%s\"}", generatedImageRepo, generatedImageRepoSecretName)

			} else {
				component.Annotations[ImageRepoAnnotationName] =
					fmt.Sprintf("{\"image\":\"%s\",\"secret\":\"\"}", generatedImageRepo)
			}
			component.Spec.ContainerImage = generatedImageRepo
			Expect(k8sClient.Update(ctx, component)).To(Succeed())

			// wait also for image finalizer on component
			// image-registry-secret-sa-link.component.appstudio.openshift.io/finalizer
			// otherwise when finalizer isn't there secret won't be unlinked from SA (when component is removed)
			waitImageRegistrySecretLinkFinalizerOnComponent(resourcePacPrepKey)

			Eventually(func() bool {
				component = getComponent(resourcePacPrepKey)
				return component.Spec.ContainerImage == generatedImageRepo
			}, timeout, interval).Should(BeTrue())

			pipelineSA := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, pipelineSAKey, pipelineSA)).To(Succeed())
			isImageRegistryGeneratedSecretLinked := false
			if pipelineSA.Secrets == nil {
				time.Sleep(1 * time.Second)
				Expect(k8sClient.Get(ctx, pipelineSAKey, pipelineSA)).To(Succeed())
			}
			for _, secret := range pipelineSA.Secrets {
				if secret.Name == generatedImageRepoSecretName {
					isImageRegistryGeneratedSecretLinked = true
					break
				}
			}
			isImageRegistryGeneratedPullSecretLinked := false
			for _, secret := range pipelineSA.ImagePullSecrets {
				if secret.Name == generatedImageRepoSecretName {
					isImageRegistryGeneratedPullSecretLinked = true
					break
				}
			}

			if secretNotEmpty {
				Expect(isImageRegistryGeneratedSecretLinked).To(BeTrue())
				Expect(isImageRegistryGeneratedPullSecretLinked).To(BeTrue())
			} else {
				Expect(isImageRegistryGeneratedSecretLinked).To(BeFalse())
				Expect(isImageRegistryGeneratedPullSecretLinked).To(BeFalse())
			}
		}

		It("should link auto generated image repository secret to pipeline service account", func() {
			assertLinkSecretToSa(true)
		})

		It("should not link auto generated image repository secret to pipeline service account, when it is linked already, also check that it keeps other secrets", func() {
			generatedImageRepoSecretName := "generated-image-repo-secret"
			serviceAccountName := types.NamespacedName{Name: buildPipelineServiceAccountName, Namespace: "default"}
			serviceAccount := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, serviceAccountName, serviceAccount)).To(Succeed())

			anotherSecretName := "anothersecret"
			// add secret to the SA
			serviceAccount.Secrets = []corev1.ObjectReference{{Name: generatedImageRepoSecretName, Namespace: "default"}, {Name: anotherSecretName, Namespace: "default"}}
			serviceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: generatedImageRepoSecretName}, {Name: anotherSecretName}}
			Expect(k8sClient.Update(ctx, serviceAccount)).Should(Succeed())

			generatedImageRepoSecretKey := types.NamespacedName{Namespace: resourcePacPrepKey.Namespace, Name: generatedImageRepoSecretName}
			// wait for secret to be removed because component was removed in afterEach
			waitSecretGone(generatedImageRepoSecretKey)
			assertLinkSecretToSa(true)

			// will remove just generate image repo secret from SA and keep anothersecret
			deleteComponent(resourcePacPrepKey)
			waitSecretGone(generatedImageRepoSecretKey)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceAccountName, serviceAccount)
				if err != nil {
					return false
				}
				return len(serviceAccount.Secrets) == 1
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Get(ctx, serviceAccountName, serviceAccount)).To(Succeed())
			Expect(len(serviceAccount.Secrets)).To(Equal(1))
			Expect(len(serviceAccount.ImagePullSecrets)).To(Equal(1))
			Expect(serviceAccount.Secrets[0].Name).To(Equal(anotherSecretName))
			Expect(serviceAccount.ImagePullSecrets[0].Name).To(Equal(anotherSecretName))

			// cleanup, remove anothersecret from the SA
			Expect(k8sClient.Get(ctx, serviceAccountName, serviceAccount)).To(Succeed())
			serviceAccount.Secrets = []corev1.ObjectReference{}
			serviceAccount.ImagePullSecrets = []corev1.LocalObjectReference{}
			Expect(k8sClient.Update(ctx, serviceAccount)).Should(Succeed())
		})

		It("should not link auto generated image repository secret to pipeline service account, when secret is missing", func() {
			// wait for secret to be removed because component was removed in afterEach
			generatedImageRepoSecretName := "generated-image-repo-secret"
			generatedImageRepoSecretKey := types.NamespacedName{Namespace: resourcePacPrepKey.Namespace, Name: generatedImageRepoSecretName}
			waitSecretGone(generatedImageRepoSecretKey)

			assertLinkSecretToSa(false)
		})
	})

	Context("Test Pipelines as Code build clean up", func() {
		var resourceCleanupKey = types.NamespacedName{Name: HASCompName + "-cleanup", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(buildServiceNamespaceName)

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
			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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
			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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
			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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
			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourceCleanupKey)
			expectPacBuildStatus(resourceCleanupKey, "enabled", 0, "", mergeUrl)

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourceCleanupKey.Name, "another")
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

			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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

			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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

			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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
			setComponentDevfileModel(resourceCleanupKey)
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

			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)
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
			createComponentAndProcessBuildRequest(resourceCleanupKey, BuildRequestConfigurePaCAnnotationValue)

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
			createNamespace(buildServiceNamespaceName)
			ResetTestGitProviderClient()

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentWithBuildRequest(anotherComponentKey, BuildRequestConfigurePaCAnnotationValue)
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
		var resouceSimpleBuildKey = types.NamespacedName{Name: HASCompName + "-simple", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(buildServiceNamespaceName)
			ResetTestGitProviderClient()

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
			createComponent(resouceSimpleBuildKey)
		})

		_ = AfterEach(func() {
			deleteSecret(pacSecretKey)

			deleteBuildPipelineRunSelector(defaultSelectorKey)
			deleteComponentPipelineRuns(resouceSimpleBuildKey)
			deleteComponent(resouceSimpleBuildKey)
			// wait for pruner operator to finish, so it won't prune runs from new test
			time.Sleep(time.Second)
			DevfileSearchForDockerfile = devfile.SearchForDockerfile
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

			setComponentDevfileModel(resouceSimpleBuildKey)

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
		})

		It("should submit initial build on component creation with sha revision", func() {
			deleteComponent(resouceSimpleBuildKey)

			gitSourceSHA := "d1a9e858489d1515621398fb02942da068f1c956"
			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.Source.GitSource.Revision = gitSourceSHA
			createComponentCustom(component)
			setComponentDevfileModel(resouceSimpleBuildKey)

			waitOneInitialPipelineRunCreated(resouceSimpleBuildKey)
			waitComponentAnnotationGone(resouceSimpleBuildKey, BuildRequestAnnotationName)
			expectSimpleBuildStatus(resouceSimpleBuildKey, 0, "", false)

			// Check pipeline run labels and annotations
			pipelineRun := listComponentPipelineRuns(resouceSimpleBuildKey)[0]
			Expect(pipelineRun.Annotations[gitCommitShaAnnotationName]).To(Equal(gitSourceSHA))
			Expect(pipelineRun.Annotations[gitRepoAtShaAnnotationName]).To(Equal(DefaultBrowseRepository + gitSourceSHA))
		})

		It("should be able to retrigger simple build", func() {
			setComponentDevfileModel(resouceSimpleBuildKey)

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

			setComponentDevfileModel(resouceSimpleBuildKey)
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

			setComponentDevfileModel(resouceSimpleBuildKey)

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

			component := getComponent(resouceSimpleBuildKey)
			component.Spec.Secret = gitSecretName
			Expect(k8sClient.Update(ctx, component)).To(Succeed())

			setComponentDevfileModel(resouceSimpleBuildKey)

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
			deleteComponent(resouceSimpleBuildKey)

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.ContainerImage = ""

			type RepositoryInfo struct {
				Image  string `json:"image"`
				Secret string `json:"secret"`
			}
			repoInfo := RepositoryInfo{Image: ComponentContainerImage, Secret: ""}
			repoInfoBytes, _ := json.Marshal(repoInfo)
			component.Annotations[ImageRepoAnnotationName] = string(repoInfoBytes)

			createComponentCustom(component)
			setComponentDevfileModel(resouceSimpleBuildKey)
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

			setComponentDevfileModel(resouceSimpleBuildKey)

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

			component := getComponent(resouceSimpleBuildKey)
			component.Spec.Secret = "git-secret"
			Expect(k8sClient.Update(ctx, component)).To(Succeed())

			setComponentDevfileModel(resouceSimpleBuildKey)

			Eventually(func() bool { return isRepositoryPublicInvoked }, timeout, interval).Should(BeTrue())

			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EComponentGitSecretMissing, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should not submit initial build if the component devfile model is not set", func() {
			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build if pac secret is missing", func() {
			deleteSecret(pacSecretKey)
			setComponentDevfileModel(resouceSimpleBuildKey)
			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should do nothing if simple build already happened (and PaC is not used)", func() {
			deleteComponent(resouceSimpleBuildKey)

			EnsurePaCMergeRequestFunc = func(repoUrl string, d *gp.MergeRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Annotations = make(map[string]string)
			component.Annotations[BuildStatusAnnotationName] = "{simple:{\"build-start-time\": \"time\"}}"
			createComponentCustom(component)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build if a gitsource is missing from component", func() {
			deleteComponent(resouceSimpleBuildKey)

			component := &appstudiov1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resouceSimpleBuildKey.Name,
					Namespace: HASAppNamespace,
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  resouceSimpleBuildKey.Name,
					Application:    HASAppName,
					ContainerImage: "quay.io/test/image:latest",
				},
			}
			createComponentCustom(component)
			setComponentDevfileModel(resouceSimpleBuildKey)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build if Dockerfile is wrong", func() {
			DevfileSearchForDockerfile = func(devfileBytes []byte) (*v1alpha2.DockerfileImage, error) {
				return nil, fmt.Errorf("wrong dockerfile")
			}
			setComponentDevfileModel(resouceSimpleBuildKey)
			waitDoneMessageOnComponent(resouceSimpleBuildKey)

			expectError := boerrors.NewBuildOpError(boerrors.EInvalidDevfile, nil)
			expectSimpleBuildStatus(resouceSimpleBuildKey, expectError.GetErrorId(), expectError.ShortError(), true)
		})

		It("should not submit initial simple build if git provider can't be detected)", func() {
			deleteComponent(resouceSimpleBuildKey)

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.Source.GitSource.URL = "wrong"
			createComponentCustom(component)
			setComponentDevfileModel(resouceSimpleBuildKey)
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

			setComponentDevfileModel(resouceSimpleBuildKey)
			Eventually(func() bool { return isRepositoryPublicInvoked }, timeout, interval).Should(BeTrue())

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build if ContainerImage is not set)", func() {
			deleteComponent(resouceSimpleBuildKey)

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Spec.ContainerImage = ""

			createComponentCustom(component)
			setComponentDevfileModel(resouceSimpleBuildKey)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})

		It("should not submit initial build when request is empty)", func() {
			deleteComponent(resouceSimpleBuildKey)

			component := getSampleComponentData(resouceSimpleBuildKey)
			component.Annotations[BuildRequestAnnotationName] = ""

			createComponentCustom(component)
			setComponentDevfileModel(resouceSimpleBuildKey)

			ensureNoPipelineRunsCreated(resouceSimpleBuildKey)
		})
	})

	Context("Resolve the correct build bundle during the component creation", func() {
		var resourceResBundleKey = types.NamespacedName{Name: HASCompName + "-resolvebundle", Namespace: HASAppNamespace}

		BeforeEach(func() {
			createNamespace(buildServiceNamespaceName)
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponent(resourceResBundleKey)
		})

		AfterEach(func() {
			deleteComponent(resourceResBundleKey)
			deleteComponentPipelineRuns(resourceResBundleKey)
			deleteSecret(pacSecretKey)
			// wait for pruner operator to finish, so it won't prune runs from new test
			time.Sleep(time.Second)
		})

		It("should use the build bundle specified for application", func() {
			selectors := &buildappstudiov1alpha1.BuildPipelineSelector{
				ObjectMeta: metav1.ObjectMeta{
					Name:      HASAppName,
					Namespace: HASAppNamespace,
				},
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name: "nodejs",
							PipelineRef: newBundleResolverPipelineRef(
								defaultPipelineBundle,
								"nodejs-builder",
							),
							PipelineParams: []buildappstudiov1alpha1.PipelineParam{
								{
									Name:  "additional-param",
									Value: "additional-param-value-application",
								},
							},
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language: "nodejs",
							},
						},
						{
							Name: "Fallback",
							PipelineRef: newBundleResolverPipelineRef(
								defaultPipelineBundle,
								"noop",
							),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, selectors)).To(Succeed())
			getComponent(resourceResBundleKey)

			devfile := `
                        schemaVersion: 2.2.0
                        metadata:
                            name: devfile-nodejs
                            language: nodejs
                    `
			setComponentDevfile(resourceResBundleKey, devfile)

			waitOneInitialPipelineRunCreated(resourceResBundleKey)
			pipelineRun := listComponentPipelineRuns(resourceResBundleKey)[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(resourceResBundleKey.Name))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal("nodejs-builder"))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(getPipelineName(pipelineRun.Spec.PipelineRef)).To(Equal("nodejs-builder"))
			Expect(getPipelineBundle(pipelineRun.Spec.PipelineRef)).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.Params).ToNot(BeNil())
			additionalPipelineParameterFound := false
			for _, param := range pipelineRun.Spec.Params {
				if param.Name == "additional-param" {
					Expect(param.Value.StringVal).To(Equal("additional-param-value-application"))
					additionalPipelineParameterFound = true
					break
				}
			}
			Expect(additionalPipelineParameterFound).To(BeTrue(), "additional pipeline parameter not found")

			Expect(k8sClient.Delete(ctx, selectors)).Should(Succeed())
		})

		It("should use the build bundle specified for the namespace", func() {
			selectors := &buildappstudiov1alpha1.BuildPipelineSelector{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildPipelineSelectorResourceName,
					Namespace: HASAppNamespace,
				},
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name: "nodejs",
							PipelineRef: newBundleResolverPipelineRef(
								defaultPipelineBundle,
								"nodejs-builder",
							),
							PipelineParams: []buildappstudiov1alpha1.PipelineParam{
								{
									Name:  "additional-param",
									Value: "additional-param-value-namespace",
								},
							},
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language: "nodejs",
							},
						},
						{
							Name: "Fallback",
							PipelineRef: newBundleResolverPipelineRef(
								defaultPipelineBundle,
								"noop",
							),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, selectors)).To(Succeed())

			devfile := `
                        schemaVersion: 2.2.0
                        metadata:
                            name: devfile-nodejs
                            language: nodejs
                    `
			setComponentDevfile(resourceResBundleKey, devfile)

			waitOneInitialPipelineRunCreated(resourceResBundleKey)
			pipelineRun := listComponentPipelineRuns(resourceResBundleKey)[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(resourceResBundleKey.Name))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal("nodejs-builder"))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(getPipelineName(pipelineRun.Spec.PipelineRef)).To(Equal("nodejs-builder"))
			Expect(getPipelineBundle(pipelineRun.Spec.PipelineRef)).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.Params).ToNot(BeNil())
			additionalPipelineParameterFound := false
			for _, param := range pipelineRun.Spec.Params {
				if param.Name == "additional-param" {
					Expect(param.Value.StringVal).To(Equal("additional-param-value-namespace"))
					additionalPipelineParameterFound = true
					break
				}
			}
			Expect(additionalPipelineParameterFound).To(BeTrue(), "additional pipeline parameter not found")

			Expect(k8sClient.Delete(ctx, selectors)).Should(Succeed())
		})

		It("should use the global build bundle", func() {
			deleteBuildPipelineRunSelector(defaultSelectorKey)
			selectors := &buildappstudiov1alpha1.BuildPipelineSelector{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildPipelineSelectorResourceName,
					Namespace: buildServiceNamespaceName,
				},
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name: "java",
							PipelineRef: newBundleResolverPipelineRef(
								defaultPipelineBundle,
								"java-builder",
							),
							PipelineParams: []buildappstudiov1alpha1.PipelineParam{
								{
									Name:  "additional-param",
									Value: "additional-param-value-global",
								},
							},
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language: "java",
							},
						},
						{
							Name: "Fallback",
							PipelineRef: newBundleResolverPipelineRef(
								defaultPipelineBundle,
								"noop",
							),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, selectors)).To(Succeed())
			getComponent(resourceResBundleKey)

			devfile := `
                        schemaVersion: 2.2.0
                        metadata:
                            name: devfile-java
                            language: java
                    `
			setComponentDevfile(resourceResBundleKey, devfile)

			waitOneInitialPipelineRunCreated(resourceResBundleKey)
			pipelineRun := listComponentPipelineRuns(resourceResBundleKey)[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(resourceResBundleKey.Name))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal("java-builder"))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(getPipelineName(pipelineRun.Spec.PipelineRef)).To(Equal("java-builder"))
			Expect(getPipelineBundle(pipelineRun.Spec.PipelineRef)).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.Params).ToNot(BeNil())
			additionalPipelineParameterFound := false
			for _, param := range pipelineRun.Spec.Params {
				if param.Name == "additional-param" {
					Expect(param.Value.StringVal).To(Equal("additional-param-value-global"))
					additionalPipelineParameterFound = true
					break
				}
			}
			Expect(additionalPipelineParameterFound).To(BeTrue(), "additional pipeline parameter not found")

			Expect(k8sClient.Delete(ctx, selectors)).Should(Succeed())
		})
	})

	Context("Test build pipeline failures related to BuildPipelineSelector", func() {
		var resourceNoMatchPKey = types.NamespacedName{Name: HASCompName + "-nomatchpipeline", Namespace: HASAppNamespace}

		BeforeEach(func() {
			deleteBuildPipelineRunSelector(defaultSelectorKey)

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
			ResetTestGitProviderClient()
		})

		AfterEach(func() {
			deleteComponent(resourceNoMatchPKey)
			deleteSecret(pacSecretKey)
		})

		createBuildPipelineSelector := func(resolverRef tektonapi.ResolverRef, whenCondition buildappstudiov1alpha1.WhenCondition) {
			selector := &buildappstudiov1alpha1.BuildPipelineSelector{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultSelectorKey.Name,
					Namespace: defaultSelectorKey.Namespace,
				},
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name:        "java",
							PipelineRef: tektonapi.PipelineRef{ResolverRef: resolverRef},
							PipelineParams: []buildappstudiov1alpha1.PipelineParam{
								{
									Name:  "additional-param",
									Value: "additional-param-value-global",
								},
							},
							WhenConditions: whenCondition,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, selector)).To(Succeed())
		}

		defaultResolverRef := tektonapi.ResolverRef{
			Resolver: "bundles",
			Params: []tektonapi.Param{
				{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
				{Name: "bundle", Value: *tektonapi.NewStructuredValues(defaultPipelineBundle)},
				{Name: "name", Value: *tektonapi.NewStructuredValues("java-builder")},
			},
		}
		nonMatchingConditions := buildappstudiov1alpha1.WhenCondition{Language: "java"}

		assertBuildFail := func(doInitialBuild bool, expectedErrMsg string) {
			component := getSampleComponentData(resourceNoMatchPKey)
			if doInitialBuild {
				component.Annotations[BuildRequestAnnotationName] = BuildRequestTriggerSimpleBuildAnnotationValue
			} else {
				component.Annotations[BuildRequestAnnotationName] = BuildRequestConfigurePaCAnnotationValue
			}
			createComponentCustom(component)
			// devfile is about nodejs rather than Java, which causes failure of selecting a pipeline.
			devfile := `
                        schemaVersion: 2.2.0
                        metadata:
                            name: devfile-nodejs
                            language: nodejs
                    `
			setComponentDevfile(resourceNoMatchPKey, devfile)

			ensureNoPipelineRunsCreated(resourceNoMatchPKey)

			component = getComponent(resourceNoMatchPKey)

			// Check expected errors
			statusAnnotation := component.Annotations[BuildStatusAnnotationName]
			Expect(strings.Contains(statusAnnotation, expectedErrMsg)).To(BeTrue())
		}

		It("initial build should fail when Component CR does not match any predefined pipeline", func() {
			createBuildPipelineSelector(defaultResolverRef, nonMatchingConditions)
			assertBuildFail(true, "No pipeline is selected")
		})

		It("PaC provision should fail when Component CR does not match any predefined pipeline", func() {
			createBuildPipelineSelector(defaultResolverRef, nonMatchingConditions)
			assertBuildFail(false, "No pipeline is selected")
		})

		It("initial build should fail when no BuildPipelineSelector CR is defined", func() {
			assertBuildFail(true, "Build pipeline selector is not defined")
		})

		It("PaC provision should fail when no BuildPipelineSelector CR is defined", func() {
			assertBuildFail(false, "Build pipeline selector is not defined")
		})

		unsupportedResolverRef := tektonapi.ResolverRef{Resolver: "git"}
		noConditions := buildappstudiov1alpha1.WhenCondition{}

		It("Initial build should fail when the matched pipelineRef uses an unsupported resolver", func() {
			createBuildPipelineSelector(unsupportedResolverRef, noConditions)
			assertBuildFail(true, "The pipelineRef for this component (based on pipeline selectors) is not supported.")
		})

		It("PaC provision should fail when the matched pipelineRef uses an unsupported resolver", func() {
			createBuildPipelineSelector(unsupportedResolverRef, noConditions)
			assertBuildFail(false, "The pipelineRef for this component (based on pipeline selectors) is not supported.")
		})

		incompleteResolverRef := tektonapi.ResolverRef{
			Resolver: "bundles",
			Params: []tektonapi.Param{
				{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
				{Name: "name", Value: *tektonapi.NewStructuredValues("java-builder")},
			},
		}

		It("Initial build should fail when the matched pipelineRef is incomplete", func() {
			createBuildPipelineSelector(incompleteResolverRef, noConditions)
			assertBuildFail(true, `The pipelineRef for this component is missing required parameters ('name' and/or 'bundle').`)
		})

		It("PaC provision should fail when the matched pipelineRef is incomplete", func() {
			createBuildPipelineSelector(incompleteResolverRef, noConditions)
			assertBuildFail(false, `The pipelineRef for this component is missing required parameters ('name' and/or 'bundle').`)
		})

	})

	Context("Test Pipelines as Code trigger build", func() {
		var resourcePacTriggerKey = types.NamespacedName{Name: HASCompName + "-pactrigger", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(buildServiceNamespaceName)
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

			createComponentAndProcessBuildRequest(resourcePacTriggerKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)
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
			createComponentAndProcessBuildRequest(resourcePacTriggerKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)
			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourcePacTriggerKey.Name, "another")
			waitPaCFinalizerOnComponent(component2Key)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)
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
			createComponentAndProcessBuildRequest(resourcePacTriggerKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)
			expectPacBuildStatus(resourcePacTriggerKey, "enabled", 0, "", mergeUrl)
			repository := waitPaCRepositoryCreated(resourcePacTriggerKey)

			// provision 2nd component
			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourcePacTriggerKey.Name, "main")
			waitPaCFinalizerOnComponent(component2Key)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)
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
			createComponentAndProcessBuildRequest(resourcePacTriggerKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)
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
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourcePacTriggerKey.Name, "another")
			waitPaCFinalizerOnComponent(component2Key)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)
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
			createComponentAndProcessBuildRequest(resourcePacTriggerKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)
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
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourcePacTriggerKey.Name, "another")
			waitPaCFinalizerOnComponent(component2Key)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)
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
			createComponentAndProcessBuildRequest(resourcePacTriggerKey, BuildRequestConfigurePaCAnnotationValue)
			waitPaCFinalizerOnComponent(resourcePacTriggerKey)
			waitComponentAnnotationGone(resourcePacTriggerKey, BuildRequestAnnotationName)
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
			createComponentWithBuildRequestAndGit(component2Key, BuildRequestConfigurePaCAnnotationValue, SampleRepoLink+"-"+resourcePacTriggerKey.Name, "another")
			waitPaCFinalizerOnComponent(component2Key)
			waitComponentAnnotationGone(component2Key, BuildRequestAnnotationName)
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
