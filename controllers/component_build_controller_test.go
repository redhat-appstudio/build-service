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
	"fmt"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/application-service/gitops"
	gitopsprepare "github.com/redhat-appstudio/application-service/gitops/prepare"
	buildappstudiov1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	"github.com/redhat-appstudio/build-service/pkg/github"
	"github.com/redhat-appstudio/build-service/pkg/gitlab"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	gogithub "github.com/google/go-github/v45/github"
	gogitlab "github.com/xanzy/go-gitlab"
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
		resourceKey           = types.NamespacedName{Name: HASCompName, Namespace: HASAppNamespace}
		pacRouteKey           = types.NamespacedName{Name: pipelinesAsCodeRouteName, Namespace: pipelinesAsCodeNamespace}
		pacSecretKey          = types.NamespacedName{Name: gitopsprepare.PipelinesAsCodeSecretName, Namespace: buildServiceNamespaceName}
		namespacePaCSecretKey = types.NamespacedName{Name: gitopsprepare.PipelinesAsCodeSecretName, Namespace: HASAppNamespace}
		webhookSecretKey      = types.NamespacedName{Name: gitops.PipelinesAsCodeWebhooksSecretName, Namespace: HASAppNamespace}
	)

	Context("Component migration", func() {
		It("should delete outdated ImageRegistrySecretLinkFinalizer finalizer from existing component", func() {
			component := getComponentData(componentConfig{})
			component.ObjectMeta.Finalizers = []string{
				ImageRegistrySecretLinkFinalizer,
			}
			Expect(controllerutil.ContainsFinalizer(component, ImageRegistrySecretLinkFinalizer)).To(BeTrue())
			Expect(k8sClient.Create(ctx, component)).To(Succeed())

			Eventually(func() bool {
				component = getComponent(resourceKey)
				return !controllerutil.ContainsFinalizer(component, ImageRegistrySecretLinkFinalizer)
			}, timeout, interval).Should(BeTrue())

			deleteComponent(resourceKey)
		})
	})

	Context("Test Pipelines as Code build preparation", func() {

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(buildServiceNamespaceName)
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			github.NewGithubClientByApp = func(appId int64, privateKeyPem []byte, owner string) (*github.GithubClient, error) { return nil, nil }
			github.NewGithubClient = func(accessToken string) *github.GithubClient { return nil }
			github.IsAppInstalledIntoRepository = func(g *github.GithubClient, owner, repository string) (bool, error) { return true, nil }
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) { return "", nil }
			github.FindUnmergedOnboardingMergeRequest = func(*github.GithubClient, string, string, string, string, string) (*gogithub.PullRequest, error) {
				return nil, nil
			}
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error { return nil }
			gitlab.EnsurePaCMergeRequest = func(g *gitlab.GitlabClient, d *gitlab.PaCMergeRequestData) (string, error) { return "", nil }
			gitlab.SetupPaCWebhook = func(g *gitlab.GitlabClient, projectPath, webhookUrl, webhookSecret string) error { return nil }
			github.UndoPaCPullRequest = func(g *github.GithubClient, d *github.PaCPullRequestData) (string, error) { return "", nil }
			github.DeletePaCWebhook = func(g *github.GithubClient, webhookUrl, owner, repository string) error { return nil }
			gitlab.UndoPaCMergeRequest = func(g *gitlab.GitlabClient, d *gitlab.PaCMergeRequestData) (string, error) { return "", nil }
			gitlab.DeletePaCWebhook = func(g *gitlab.GitlabClient, projectPath, webhookUrl string) error { return nil }

			createComponentForPaCBuild(getSampleComponentData(resourceKey))
		})

		_ = AfterEach(func() {
			deleteComponent(resourceKey)

			deleteSecret(webhookSecretKey)
			deleteSecret(namespacePaCSecretKey)

			deleteSecret(pacSecretKey)
			deleteRoute(pacRouteKey)
		})

		It("should successfully submit PR with PaC definitions using GitHub application and set PaC annotation", func() {
			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(d.Owner).To(Equal("devfile-samples"))
				Expect(d.Repository).To(Equal("devfile-sample-java-springboot-basic"))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.Branch).ToNot(BeEmpty())
				Expect(d.BaseBranch).To(Equal("main"))
				Expect(d.PRTitle).ToNot(BeEmpty())
				Expect(d.PRText).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error {
				defer GinkgoRecover()
				Fail("Should not create webhook if GitHub application is used")
				return nil
			}

			setComponentDevfileModel(resourceKey)

			waitPaCRepositoryCreated(resourceKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
		})

		It("should fail to submit PR if GitHub application is not installed into repo", func() {
			github.IsAppInstalledIntoRepository = func(*github.GithubClient, string, string) (bool, error) {
				return false, nil
			}

			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(*github.GithubClient, *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}

			setComponentDevfileModel(resourceKey)

			waitComponentAnnotationValue(resourceKey,
				PaCProvisionAnnotationName, PaCProvisionErrorAnnotationValue)
			expectedErr := boerrors.NewBuildOpError(
				boerrors.EGitHubAppNotInstalled, fmt.Errorf("something is wrong"))
			waitComponentAnnotationValue(resourceKey,
				PaCProvisionErrorDetailsAnnotationName, expectedErr.ShortError())

			Expect(isCreatePaCPullRequestInvoked).Should(BeFalse())
		})

		It("should fail to submit PR if unknown git provider is used", func() {
			component := getComponent(resourceKey)
			invalidGitUrl := strings.Replace(SampleRepoLink, "github.com", "outer-space.git", 1)
			component.Spec.Source.GitSource.URL = invalidGitUrl
			Expect(k8sClient.Update(ctx, component)).To(Succeed())
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, resourceKey, component)).To(Succeed())
				return component.Spec.Source.GitSource.URL == invalidGitUrl
			}, timeout, interval).Should(BeTrue())

			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(*github.GithubClient, *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}

			setComponentDevfileModel(resourceKey)

			waitComponentAnnotationValue(resourceKey,
				PaCProvisionAnnotationName, PaCProvisionErrorAnnotationValue)
			expectedErr := boerrors.NewBuildOpError(
				boerrors.EUnknownGitProvider, fmt.Errorf("something is wrong"))
			waitComponentAnnotationValue(resourceKey,
				PaCProvisionErrorDetailsAnnotationName, expectedErr.ShortError())

			Expect(isCreatePaCPullRequestInvoked).Should(BeFalse())
		})

		It("should fail to submit PR if PaC secret is invalid", func() {
			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(*github.GithubClient, *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}

			deleteSecret(pacSecretKey)

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    "secret private key",
			}
			createSecret(pacSecretKey, pacSecretData)
			var pacSecret corev1.Secret
			Eventually(func() error {
				return k8sClient.Get(ctx, pacSecretKey, &pacSecret)
			}, timeout, interval).Should(Succeed())

			setComponentDevfileModel(resourceKey)

			waitComponentAnnotationValue(resourceKey,
				PaCProvisionAnnotationName, PaCProvisionErrorAnnotationValue)
			expectedErr := boerrors.NewBuildOpError(
				boerrors.EPaCSecretInvalid, fmt.Errorf("something is wrong"))
			waitComponentAnnotationValue(resourceKey,
				PaCProvisionErrorDetailsAnnotationName, expectedErr.ShortError())

			Expect(isCreatePaCPullRequestInvoked).Should(BeFalse())
		})

		It("should successfully submit PR with PaC definitions using GitHub token and set PaC annotation", func() {
			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(d.Owner).To(Equal("devfile-samples"))
				Expect(d.Repository).To(Equal("devfile-sample-java-springboot-basic"))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.Branch).ToNot(BeEmpty())
				Expect(d.BaseBranch).ToNot(BeEmpty())
				Expect(d.PRTitle).ToNot(BeEmpty())
				Expect(d.PRText).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			isSetupPaCWebhookInvoked := false
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error {
				isSetupPaCWebhookInvoked = true
				Expect(webhookUrl).To(Equal(pacWebhookUrl))
				Expect(webhookSecret).ToNot(BeEmpty())
				Expect(owner).To(Equal("devfile-samples"))
				Expect(repository).To(Equal("devfile-sample-java-springboot-basic"))
				return nil
			}

			pacSecretData := map[string]string{"github.token": "ghp_token"}
			createSecret(pacSecretKey, pacSecretData)

			setComponentDevfileModel(resourceKey)

			waitSecretCreated(namespacePaCSecretKey)
			waitSecretCreated(webhookSecretKey)
			waitPaCRepositoryCreated(resourceKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isSetupPaCWebhookInvoked
			}, timeout, interval).Should(BeTrue())
			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
		})

		It("should successfully submit MR with PaC definitions using GitLab token and set PaC annotation", func() {
			isCreatePaCPullRequestInvoked := false
			gitlab.EnsurePaCMergeRequest = func(c *gitlab.GitlabClient, d *gitlab.PaCMergeRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				Expect(d.ProjectPath).To(Equal("devfile-samples/devfile-sample-go-basic"))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.Branch).ToNot(BeEmpty())
				Expect(d.BaseBranch).ToNot(BeEmpty())
				Expect(d.MrTitle).ToNot(BeEmpty())
				Expect(d.MrText).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			isSetupPaCWebhookInvoked := false
			gitlab.SetupPaCWebhook = func(g *gitlab.GitlabClient, projectPath, webhookUrl, webhookSecret string) error {
				isSetupPaCWebhookInvoked = true
				Expect(webhookUrl).To(Equal(pacWebhookUrl))
				Expect(webhookSecret).ToNot(BeEmpty())
				Expect(projectPath).To(Equal("devfile-samples/devfile-sample-go-basic"))
				return nil
			}

			pacSecretData := map[string]string{"gitlab.token": "glpat-token"}
			createSecret(pacSecretKey, pacSecretData)

			deleteComponent(resourceKey)

			component := getSampleComponentData(resourceKey)
			component.Annotations = map[string]string{
				PaCProvisionAnnotationName: PaCProvisionRequestedAnnotationValue,
			}
			component.Spec.Source.GitSource.URL = "https://gitlab.com/devfile-samples/devfile-sample-go-basic"
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			setComponentDevfileModel(resourceKey)

			waitSecretCreated(namespacePaCSecretKey)
			waitSecretCreated(webhookSecretKey)
			waitPaCRepositoryCreated(resourceKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isSetupPaCWebhookInvoked
			}, timeout, interval).Should(BeTrue())
			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
		})

		It("should provision PaC definitions after initial build if PaC annotation added", func() {
			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error {
				defer GinkgoRecover()
				Fail("Should not create webhook if GitHub application is used")
				return nil
			}

			deleteComponent(resourceKey)

			createComponent(resourceKey)
			setComponentDevfileModel(resourceKey)
			waitOneInitialPipelineRunCreated(resourceKey)
			ensureComponentInitialBuildAnnotationState(resourceKey, true)

			// Add PaC annotation
			component := getComponent(resourceKey)
			if len(component.Annotations) == 0 {
				component.Annotations = make(map[string]string)
			}
			component.Annotations[PaCProvisionAnnotationName] = PaCProvisionRequestedAnnotationValue
			Expect(k8sClient.Update(ctx, component)).To(Succeed())

			waitPaCRepositoryCreated(resourceKey)
			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
		})

		It("should not copy PaC secret into local namespace if GitHub application is used", func() {
			deleteSecret(namespacePaCSecretKey)

			setComponentDevfileModel(resourceKey)

			waitPaCRepositoryCreated(resourceKey)

			ensureSecretNotCreated(namespacePaCSecretKey)
		})

		It("should reuse the same webhook secret for multicomponent repository", func() {
			var webhookSecretStrings []string
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error {
				webhookSecretStrings = append(webhookSecretStrings, webhookSecret)
				return nil
			}

			pacSecretData := map[string]string{"github.token": "ghp_token"}
			createSecret(pacSecretKey, pacSecretData)

			component1Key := resourceKey

			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			component2 := getSampleComponentData(component2Key)
			component2.Annotations = map[string]string{
				PaCProvisionAnnotationName: PaCProvisionRequestedAnnotationValue,
			}
			component2.Spec.ContainerImage = "registry.io/username/image2:tag2"
			Expect(k8sClient.Create(ctx, component2)).Should(Succeed())
			defer deleteComponent(component2Key)

			setComponentDevfileModel(component1Key)
			setComponentDevfileModel(component2Key)

			waitSecretCreated(namespacePaCSecretKey)
			waitSecretCreated(webhookSecretKey)

			waitPaCRepositoryCreated(component1Key)
			waitPaCRepositoryCreated(component2Key)

			waitComponentAnnotationValue(component1Key, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
			waitComponentAnnotationValue(component2Key, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)

			Expect(len(webhookSecretStrings) > 0).To(BeTrue())
			for _, webhookSecret := range webhookSecretStrings {
				Expect(webhookSecret).To(Equal(webhookSecretStrings[0]))
			}
		})

		It("should use different webhook secrets for different components of the same application", func() {
			var webhookSecretStrings []string
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error {
				webhookSecretStrings = append(webhookSecretStrings, webhookSecret)
				return nil
			}

			pacSecretData := map[string]string{"github.token": "ghp_token"}
			createSecret(pacSecretKey, pacSecretData)

			component1Key := resourceKey

			component2Key := types.NamespacedName{Name: "component2", Namespace: HASAppNamespace}
			component2 := getSampleComponentData(component2Key)
			component2.Annotations = map[string]string{
				PaCProvisionAnnotationName: PaCProvisionRequestedAnnotationValue,
			}
			component2.Spec.ContainerImage = "registry.io/username/image2:tag2"
			component2.Spec.Source.GitSource.URL = "https://github.com/devfile-samples/devfile-sample-go-basic"
			Expect(k8sClient.Create(ctx, component2)).Should(Succeed())
			defer deleteComponent(component2Key)

			setComponentDevfileModel(component1Key)
			setComponentDevfileModel(component2Key)

			waitSecretCreated(namespacePaCSecretKey)
			waitSecretCreated(webhookSecretKey)

			waitPaCRepositoryCreated(component1Key)
			waitPaCRepositoryCreated(component2Key)

			waitComponentAnnotationValue(component1Key, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
			waitComponentAnnotationValue(component2Key, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)

			Expect(len(webhookSecretStrings)).To(Equal(2))
			Expect(webhookSecretStrings[0]).ToNot(Equal(webhookSecretStrings[1]))
		})

		It("should not set PaC annotation if PaC definitions PR submission failed", func() {
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				return "", fmt.Errorf("Failed to submit PaC definitions PR")
			}

			setComponentDevfileModel(resourceKey)

			waitPaCRepositoryCreated(resourceKey)

			ensureComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionRequestedAnnotationValue)

			// Clean up after the test
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				return "", nil
			}
			deleteComponent(resourceKey)
			// Wait a bit to not to spoil the next tests.
			// The reason for this is that the test operator could be in the middle of reconcile loop
			// and it's needed to wait until execution reaches the end of the reconcile loop.
			time.Sleep(time.Second)
		})

		It("should successfully do PaC provision after error (when PaC GitHub Application was not installed)", func() {
			appNotInstalledErr := boerrors.NewBuildOpError(boerrors.EGitHubAppNotInstalled, nil)
			github.NewGithubClientByApp = func(appId int64, privateKeyPem []byte, owner string) (*github.GithubClient, error) {
				return nil, appNotInstalledErr
			}
			github.NewGithubClient = func(accessToken string) *github.GithubClient {
				defer GinkgoRecover()
				Fail("GitHub client should be created using PaC GitHub Application")
				return nil
			}
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			setComponentDevfileModel(resourceKey)
			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionErrorAnnotationValue)
			waitComponentAnnotationValue(resourceKey, PaCProvisionErrorDetailsAnnotationName, appNotInstalledErr.ShortError())

			// Ensure no more retries after permanent error
			github.NewGithubClientByApp = func(appId int64, privateKeyPem []byte, owner string) (*github.GithubClient, error) {
				defer GinkgoRecover()
				Fail("Should not retry PaC provision on permanent error")
				return nil, nil
			}
			ensureComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionErrorAnnotationValue)

			// Suppose PaC GH App is installed
			github.NewGithubClientByApp = func(appId int64, privateKeyPem []byte, owner string) (*github.GithubClient, error) {
				return nil, nil
			}
			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}
			// Update PaC annotation to retry
			component := getComponent(resourceKey)
			component.Annotations[PaCProvisionAnnotationName] = PaCProvisionRequestedAnnotationValue
			Expect(k8sClient.Update(ctx, component)).Should(Succeed())

			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, PaCProvisionDoneAnnotationValue)
		})

		It("should not submit PaC definitions PR if PaC secret is missing", func() {
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			deleteSecret(pacSecretKey)
			deleteSecret(namespacePaCSecretKey)

			setComponentDevfileModel(resourceKey)

			waitComponentAnnotationValue(resourceKey, PaCProvisionAnnotationName, "error")
		})

		It("should do nothing if the component devfile model is not set", func() {
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			ensureComponentInitialBuildAnnotationState(resourceKey, false)
		})

		It("should do nothing if the component image is not defined in spec nor annotation", func() {
			deleteComponent(resourceKey)
			component := getComponentData(componentConfig{})
			component.Spec.ContainerImage = ""
			Expect(k8sClient.Create(ctx, component)).To(Succeed())
			setComponentDevfileModel(resourceKey)

			ensureComponentInitialBuildAnnotationState(resourceKey, false)
		})

		It("should do nothing if initial build annotation is already set", func() {
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			component := getComponent(resourceKey)
			component.Annotations = make(map[string]string)
			component.Annotations[InitialBuildAnnotationName] = "true"
			Expect(k8sClient.Update(ctx, component)).Should(Succeed())

			setComponentDevfileModel(resourceKey)

			time.Sleep(ensureTimeout)
		})

		It("should do nothing if a container image source is specified in component", func() {
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("PR creation should not be invoked")
				return "", nil
			}

			deleteComponent(resourceKey)
			component := &appstudiov1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      HASCompName,
					Namespace: HASAppNamespace,
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  HASCompName,
					Application:    HASAppName,
					ContainerImage: "quay.io/test/image:latest",
				},
			}
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			setComponentDevfileModel(resourceKey)

			ensureComponentInitialBuildAnnotationState(resourceKey, false)
		})

		It("should set default branch as base branch properly when Revision is not set", func() {
			deleteComponent(resourceKey)

			sampleComponent := getSampleComponentData(resourceKey)
			// Unset Revision so that GetDefaultBranch function is called to use the default branch
			// set in the remote component repository
			sampleComponent.Spec.Source.GitSource.Revision = ""
			createComponentForPaCBuild(sampleComponent)

			const repoDefaultBranch = "cool-feature"

			github.GetDefaultBranch = func(*github.GithubClient, string, string) (string, error) {
				return repoDefaultBranch, nil
			}

			isCreatePaCPullRequestInvoked := false
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				Expect(d.BaseBranch).To(Equal(repoDefaultBranch))
				for _, file := range d.Files {
					var prYaml v1beta1.PipelineRun
					if err := yaml.Unmarshal(file.Content, &prYaml); err != nil {
						return "", err
					}
					targetBranches := prYaml.Annotations["pipelinesascode.tekton.dev/on-target-branch"]
					Expect(targetBranches).To(Equal(fmt.Sprintf("[%s]", repoDefaultBranch)))
				}
				isCreatePaCPullRequestInvoked = true
				return "url", nil
			}

			setComponentDevfileModel(resourceKey)

			Eventually(func() bool {
				return isCreatePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Test Pipelines as Code build clean up", func() {

		_ = BeforeEach(func() {
			createNamespace(pipelinesAsCodeNamespace)
			createRoute(pacRouteKey, "pac-host")
			createNamespace(buildServiceNamespaceName)

			github.NewGithubClientByApp = func(appId int64, privateKeyPem []byte, owner string) (*github.GithubClient, error) { return nil, nil }
			github.NewGithubClient = func(accessToken string) *github.GithubClient { return nil }
			github.CreatePaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) { return "", nil }
			github.SetupPaCWebhook = func(g *github.GithubClient, webhookUrl, webhookSecret, owner, repository string) error { return nil }
			github.IsAppInstalledIntoRepository = func(g *github.GithubClient, owner, repository string) (bool, error) { return true, nil }
			github.FindUnmergedOnboardingMergeRequest = func(*github.GithubClient, string, string, string, string, string) (*gogithub.PullRequest, error) {
				return nil, nil
			}
			gitlab.EnsurePaCMergeRequest = func(g *gitlab.GitlabClient, d *gitlab.PaCMergeRequestData) (string, error) { return "", nil }
			gitlab.SetupPaCWebhook = func(g *gitlab.GitlabClient, projectPath, webhookUrl, webhookSecret string) error { return nil }
			gitlab.FindUnmergedOnboardingMergeRequest = func(*gitlab.GitlabClient, string, string, string, string) (*gogitlab.MergeRequest, error) {
				return nil, nil
			}
		})

		_ = AfterEach(func() {
			deleteSecret(webhookSecretKey)
			deleteSecret(namespacePaCSecretKey)

			deleteSecret(pacSecretKey)
			deleteRoute(pacRouteKey)
		})

		It("should successfully submit PR with PaC definitions removal using GitHub application", func() {
			isRemovePaCPullRequestInvoked := false
			github.UndoPaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				isRemovePaCPullRequestInvoked = true
				Expect(d.Owner).To(Equal("devfile-samples"))
				Expect(d.Repository).To(Equal("devfile-sample-java-springboot-basic"))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.Branch).ToNot(BeEmpty())
				Expect(d.BaseBranch).ToNot(BeEmpty())
				Expect(d.PRTitle).ToNot(BeEmpty())
				Expect(d.PRText).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			github.DeletePaCWebhook = func(g *github.GithubClient, webhookUrl, owner, repository string) error {
				defer GinkgoRecover()
				Fail("Should not try to delete webhook if GitHub application is used")
				return nil
			}

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentForPaCBuild(getSampleComponentData(resourceKey))
			setComponentDevfileModel(resourceKey)
			waitPaCFinalizerOnComponent(resourceKey)

			deleteComponent(resourceKey)

			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
		})

		It("should successfully submit PR with PaC definitions removal using GitHub token", func() {
			isRemovePaCPullRequestInvoked := false
			github.UndoPaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				isRemovePaCPullRequestInvoked = true
				Expect(d.Owner).To(Equal("devfile-samples"))
				Expect(d.Repository).To(Equal("devfile-sample-java-springboot-basic"))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.Branch).ToNot(BeEmpty())
				Expect(d.BaseBranch).ToNot(BeEmpty())
				Expect(d.PRTitle).ToNot(BeEmpty())
				Expect(d.PRText).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			isDeletePaCWebhookInvoked := false
			github.DeletePaCWebhook = func(g *github.GithubClient, webhookUrl, owner, repository string) error {
				isDeletePaCWebhookInvoked = true
				Expect(webhookUrl).To(Equal(pacWebhookUrl))
				Expect(owner).To(Equal("devfile-samples"))
				Expect(repository).To(Equal("devfile-sample-java-springboot-basic"))
				return nil
			}

			pacSecretData := map[string]string{"github.token": "ghp_token"}
			createSecret(pacSecretKey, pacSecretData)

			createComponentForPaCBuild(getSampleComponentData(resourceKey))
			setComponentDevfileModel(resourceKey)
			waitPaCFinalizerOnComponent(resourceKey)

			deleteComponent(resourceKey)

			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isDeletePaCWebhookInvoked
			}, timeout, interval).Should(BeTrue())
		})

		It("should successfully submit MR with PaC definitions removal using GitLab token", func() {
			isRemovePaCPullRequestInvoked := false
			gitlab.UndoPaCMergeRequest = func(c *gitlab.GitlabClient, d *gitlab.PaCMergeRequestData) (string, error) {
				isRemovePaCPullRequestInvoked = true
				Expect(d.ProjectPath).To(Equal("devfile-samples/devfile-sample-go-basic"))
				Expect(len(d.Files)).To(Equal(2))
				for _, file := range d.Files {
					Expect(strings.HasPrefix(file.FullPath, ".tekton/")).To(BeTrue())
				}
				Expect(d.CommitMessage).ToNot(BeEmpty())
				Expect(d.Branch).ToNot(BeEmpty())
				Expect(d.BaseBranch).ToNot(BeEmpty())
				Expect(d.MrTitle).ToNot(BeEmpty())
				Expect(d.MrText).ToNot(BeEmpty())
				Expect(d.AuthorName).ToNot(BeEmpty())
				Expect(d.AuthorEmail).ToNot(BeEmpty())
				return "url", nil
			}
			isDeletePaCWebhookInvoked := false
			gitlab.DeletePaCWebhook = func(g *gitlab.GitlabClient, projectPath, webhookUrl string) error {
				isDeletePaCWebhookInvoked = true
				Expect(webhookUrl).To(Equal(pacWebhookUrl))
				Expect(projectPath).To(Equal("devfile-samples/devfile-sample-go-basic"))
				return nil
			}
			isDeleteBranchInvoked := false
			gitlab.DeleteBranch = func(*gitlab.GitlabClient, string, string) error {
				isDeleteBranchInvoked = true
				return nil
			}

			pacSecretData := map[string]string{"gitlab.token": "glpat-token"}
			createSecret(pacSecretKey, pacSecretData)

			component := getSampleComponentData(resourceKey)
			component.Annotations = map[string]string{
				PaCProvisionAnnotationName: PaCProvisionRequestedAnnotationValue,
			}
			component.Spec.Source.GitSource.URL = "https://gitlab.com/devfile-samples/devfile-sample-go-basic"
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())
			setComponentDevfileModel(resourceKey)
			waitPaCFinalizerOnComponent(resourceKey)

			deleteComponent(resourceKey)

			Eventually(func() bool {
				return isRemovePaCPullRequestInvoked
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isDeletePaCWebhookInvoked
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return isDeleteBranchInvoked
			}, timeout, interval).Should(BeFalse())
		})

		It("should not block component deletion if PaC definitions removal failed", func() {
			github.UndoPaCPullRequest = func(c *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				return "", fmt.Errorf("failed to create PR")
			}
			github.DeletePaCWebhook = func(g *github.GithubClient, webhookUrl, owner, repository string) error {
				return fmt.Errorf("failed to delete webhook")
			}

			pacSecretData := map[string]string{"github.token": "ghp_token"}
			createSecret(pacSecretKey, pacSecretData)

			createComponentForPaCBuild(getSampleComponentData(resourceKey))
			setComponentDevfileModel(resourceKey)
			waitPaCFinalizerOnComponent(resourceKey)

			deleteComponent(resourceKey)
		})

		var assertCloseUnmergedPullRequest = func(expectedBaseBranch string, sourceBranchExists bool) {
			github.UndoPaCPullRequest = func(g *github.GithubClient, d *github.PaCPullRequestData) (string, error) {
				defer GinkgoRecover()
				Fail("Expect to close unmerged merge request other than delete .tekton/")
				return "", nil
			}

			component := getSampleComponentData(resourceKey)
			if expectedBaseBranch == "" {
				expectedBaseBranch = component.Spec.Source.GitSource.Revision
			} else {
				component.Spec.Source.GitSource.Revision = ""
			}
			gitUrl := strings.TrimSuffix(component.Spec.Source.GitSource.URL, ".git")
			gitSourceUrlParts := strings.Split(gitUrl, "/")
			pullRequestUrl := fmt.Sprintf("%s/pull/1", gitUrl)

			expectedSourceBranch := pacMergeRequestSourceBranchPrefix + component.Name

			isFindOnboardingMergeRequestInvoked := false
			github.FindUnmergedOnboardingMergeRequest = func(
				client *github.GithubClient, owner, repository, sourceBranch, baseBranch, authorName string) (*gogithub.PullRequest, error) {
				isFindOnboardingMergeRequestInvoked = true
				Expect(sourceBranch).Should(Equal(expectedSourceBranch))
				Expect(baseBranch).Should(Equal(expectedBaseBranch))
				Expect(authorName).Should(Equal(owner))
				Expect(gitSourceUrlParts[3]).Should(Equal(owner))
				Expect(gitSourceUrlParts[4]).Should(Equal(repository))
				return &gogithub.PullRequest{
					URL: gogithub.String(pullRequestUrl),
				}, nil
			}

			isDeleteBranchInvoked := false
			github.DeleteBranch = func(client *github.GithubClient, owner, repository, branch string) error {
				isDeleteBranchInvoked = true
				Expect(branch).Should(Equal(expectedSourceBranch))
				Expect(gitSourceUrlParts[3]).Should(Equal(owner))
				Expect(gitSourceUrlParts[4]).Should(Equal(repository))
				if !sourceBranchExists {
					return &gogithub.ErrorResponse{
						Response: &http.Response{
							StatusCode: 422,
						},
					}
				}
				return nil
			}

			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)

			createComponentForPaCBuild(component)
			setComponentDevfileModel(resourceKey)
			waitPaCFinalizerOnComponent(resourceKey)

			deleteComponent(resourceKey)

			Eventually(func() bool {
				return isFindOnboardingMergeRequestInvoked
			}, timeout, interval).Should(BeTrue(),
				"github.FindUnmergedOnboardingMergeRequest should be invoked, but not.")
			Eventually(func() bool {
				return isDeleteBranchInvoked
			}, timeout, interval).Should(BeTrue(),
				"github.DeleteBranch should be invoked, but not.")
		}

		It("should close unmerged PaC pull request using GitHub application, opened based on branch specified in Revision", func() {
			assertCloseUnmergedPullRequest("", true)
		})

		It("should keep silent when close unmerged PaC pull request using GitHub app by deleting a non-existing source branch", func() {
			assertCloseUnmergedPullRequest("", false)
		})

		It("should close unmerged PaC pull request using GitHub application, opened based on default branch", func() {
			defaultBranch := "devel"
			github.GetDefaultBranch = func(*github.GithubClient, string, string) (string, error) {
				return defaultBranch, nil
			}
			assertCloseUnmergedPullRequest(defaultBranch, true)
		})

		var assertCloseUnmergedMR = func(expectedBaseBranch string, sourceBranchExists bool) {
			const gitUrl = "https://gitlab.com/devfile-samples/devfile-sample-go-basic"
			const expectedProjectPath = "devfile-samples/devfile-sample-go-basic"
			component := getSampleComponentData(resourceKey)
			component.Annotations = map[string]string{
				PaCProvisionAnnotationName: PaCProvisionRequestedAnnotationValue,
			}
			component.Spec.Source.GitSource.URL = gitUrl
			if expectedBaseBranch == "" {
				expectedBaseBranch = component.Spec.Source.GitSource.Revision
			} else {
				component.Spec.Source.GitSource.Revision = ""
			}
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			mrUrl := fmt.Sprintf("%s/-/merge_requests/1", gitUrl)
			existingMR := &gogitlab.MergeRequest{
				ID:     1,
				WebURL: mrUrl,
			}

			isUndoPaCMergeRequestInvoked := false
			gitlab.UndoPaCMergeRequest = func(g *gitlab.GitlabClient, d *gitlab.PaCMergeRequestData) (string, error) {
				isUndoPaCMergeRequestInvoked = true
				return "", nil
			}

			gitlab.FindUnmergedOnboardingMergeRequest = func(
				g *gitlab.GitlabClient, projectPath, sourceBranch, baseBranch, authorName string) (*gogitlab.MergeRequest, error) {
				Expect(sourceBranch).Should(Equal(fmt.Sprintf("appstudio-%s", component.Name)))
				Expect(projectPath).Should(Equal(expectedProjectPath))
				Expect(baseBranch).Should(Equal(expectedBaseBranch))
				Expect(authorName).Should(Equal("redhat-appstudio"))
				return existingMR, nil
			}

			isDeleteBranchInvoked := false
			gitlab.DeleteBranch = func(g *gitlab.GitlabClient, projectPath, sourceBranch string) error {
				isDeleteBranchInvoked = true
				Expect(projectPath).Should(Equal(expectedProjectPath))
				Expect(sourceBranch).Should(Equal(pacMergeRequestSourceBranchPrefix + component.Name))
				if !sourceBranchExists {
					return &gogitlab.ErrorResponse{
						Response: &http.Response{
							StatusCode: 404,
						},
					}
				}
				return nil
			}

			pacSecretData := map[string]string{"gitlab.token": "glpat-token"}
			createSecret(pacSecretKey, pacSecretData)

			setComponentDevfileModel(resourceKey)
			waitPaCFinalizerOnComponent(resourceKey)

			deleteComponent(resourceKey)

			Eventually(func() bool {
				return isUndoPaCMergeRequestInvoked
			}, timeout, interval).Should(BeFalse(),
				"gitlab.UndoPaCMergeRequest should not be invoked, but it was invoked.")
			Eventually(func() bool {
				return isDeleteBranchInvoked
			}, timeout, interval).Should(BeTrue(),
				"gitlab.DeleteBranch should be invoked, but not.")
		}

		It("should close unmerged PaC merge request using GitLab token, opened based on branch specified in Revision", func() {
			assertCloseUnmergedMR("", true)
		})

		It("should keep silent when close unmerged PaC merge request using GitLab token by deleting a non-existing source branch", func() {
			assertCloseUnmergedMR("", false)
		})

		It("should close unmerged PaC merge request using GitLab token, opened based on default branch", func() {
			defaultBranch := "devel"
			gitlab.GetDefaultBranch = func(*gitlab.GitlabClient, string) (string, error) {
				return defaultBranch, nil
			}
			assertCloseUnmergedMR(defaultBranch, true)
		})
	})

	Context("Test initial build", func() {

		_ = BeforeEach(func() {
			createComponent(resourceKey)
		})

		_ = AfterEach(func() {
			deleteComponentPipelineRuns(resourceKey)
			deleteComponent(resourceKey)
		})

		It("should submit initial build", func() {
			setComponentDevfileModel(resourceKey)

			waitOneInitialPipelineRunCreated(resourceKey)
			ensureComponentInitialBuildAnnotationState(resourceKey, true)
		})

		It("should submit initial build for private git repository", func() {
			deleteComponent(resourceKey)

			gitSecretKey := types.NamespacedName{Name: GitSecretName, Namespace: resourceKey.Namespace}
			gitSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitSecretKey.Name,
					Namespace: gitSecretKey.Namespace,
				},
			}
			Expect(k8sClient.Create(ctx, gitSecret)).Should(Succeed())

			// Create component that refers to the git secret
			component := getSampleComponentData(resourceKey)
			component.Spec.ContainerImage = "docker.io/foo/customized:default-test-component"
			component.Spec.Secret = GitSecretName
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())
			setComponentDevfileModel(resourceKey)

			// Wait until all resources created
			waitOneInitialPipelineRunCreated(resourceKey)
			ensureComponentInitialBuildAnnotationState(resourceKey, true)

			// Check that git credentials secret is annotated
			Expect(k8sClient.Get(ctx, gitSecretKey, gitSecret)).Should(Succeed())
			tektonGitAnnotation := gitSecret.ObjectMeta.Annotations["tekton.dev/git-0"]
			Expect(tektonGitAnnotation).To(Equal("https://github.com"))

			// Check that the pipeline service account has been linked with the Github authentication credentials
			var pipelineSA corev1.ServiceAccount
			pipelineSAKey := types.NamespacedName{Namespace: resourceKey.Namespace, Name: buildPipelineServiceAccountName}
			Expect(k8sClient.Get(ctx, pipelineSAKey, &pipelineSA)).Should(Succeed())

			secretFound := false
			for _, secret := range pipelineSA.Secrets {
				if secret.Name == GitSecretName {
					secretFound = true
					break
				}
			}
			Expect(secretFound).To(BeTrue())

			// Check the pipeline run and its resources
			pipelineRuns := listComponentPipelineRuns(resourceKey)
			Expect(len(pipelineRuns)).To(Equal(1))
			pipelineRun := pipelineRuns[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(HASCompName))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal(defaultPipelineName))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(pipelineRun.Spec.PipelineRef.Name).To(Equal(defaultPipelineName))
			Expect(pipelineRun.Spec.PipelineRef.Bundle).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.Params).ToNot(BeEmpty())
			for _, p := range pipelineRun.Spec.Params {
				switch p.Name {
				case "output-image":
					Expect(p.Value.StringVal).ToNot(BeEmpty())
					Expect(strings.HasPrefix(p.Value.StringVal, "docker.io/foo/customized:"+HASCompName+"-build-"))
				case "git-url":
					Expect(p.Value.StringVal).To(Equal(SampleRepoLink))
				case "revision":
					Expect(p.Value.StringVal).To(Equal("main"))
				}
			}

			Expect(pipelineRun.Spec.Workspaces).To(Not(BeEmpty()))
			for _, w := range pipelineRun.Spec.Workspaces {
				if w.Name == "workspace" {
					Expect(w.VolumeClaimTemplate).NotTo(
						Equal(nil), "PipelineRun should have its own volumeClaimTemplate.")
				}
			}

			// Clean up
			deleteSecret(gitSecretKey)
		})

		It("should not submit initial build if the component devfile model is not set", func() {
			ensureNoPipelineRunsCreated(resourceKey)
		})

		It("should not submit initial build if initial build annotation on the component is set", func() {
			component := getComponent(resourceKey)
			component.Annotations = make(map[string]string)
			component.Annotations[InitialBuildAnnotationName] = "processed"
			Expect(k8sClient.Update(ctx, component)).Should(Succeed())

			setComponentDevfileModel(resourceKey)

			ensureNoPipelineRunsCreated(resourceKey)
		})

		It("should not submit initial build if a container image source is specified in component", func() {
			deleteComponent(resourceKey)

			component := &appstudiov1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      HASCompName,
					Namespace: HASAppNamespace,
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  HASCompName,
					Application:    HASAppName,
					ContainerImage: "quay.io/test/image:latest",
				},
			}
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			setComponentDevfileModel(resourceKey)

			ensureNoPipelineRunsCreated(resourceKey)
		})

	})

	Context("Resolve the correct build bundle during the component's creation", func() {

		BeforeEach(func() {
			createNamespace(buildServiceNamespaceName)

			createComponent(resourceKey)
		})

		AfterEach(func() {
			deleteComponent(resourceKey)
			deleteComponentPipelineRuns(resourceKey)
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
							PipelineRef: v1beta1.PipelineRef{
								Name:   "nodejs-builder",
								Bundle: defaultPipelineBundle,
							},
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
							PipelineRef: v1beta1.PipelineRef{
								Name:   "noop",
								Bundle: defaultPipelineBundle,
							},
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
			setComponentDevfile(resourceKey, devfile)

			waitOneInitialPipelineRunCreated(resourceKey)
			pipelineRun := listComponentPipelineRuns(resourceKey)[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(HASCompName))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal("nodejs-builder"))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(pipelineRun.Spec.PipelineRef.Name).To(Equal("nodejs-builder"))
			Expect(pipelineRun.Spec.PipelineRef.Bundle).To(Equal(defaultPipelineBundle))

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
							PipelineRef: v1beta1.PipelineRef{
								Name:   "nodejs-builder",
								Bundle: defaultPipelineBundle,
							},
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
							PipelineRef: v1beta1.PipelineRef{
								Name:   "noop",
								Bundle: defaultPipelineBundle,
							},
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
			setComponentDevfile(resourceKey, devfile)

			waitOneInitialPipelineRunCreated(resourceKey)
			pipelineRun := listComponentPipelineRuns(resourceKey)[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(HASCompName))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal("nodejs-builder"))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(pipelineRun.Spec.PipelineRef.Name).To(Equal("nodejs-builder"))
			Expect(pipelineRun.Spec.PipelineRef.Bundle).To(Equal(defaultPipelineBundle))

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
			selectors := &buildappstudiov1alpha1.BuildPipelineSelector{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildPipelineSelectorResourceName,
					Namespace: buildServiceNamespaceName,
				},
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name: "java",
							PipelineRef: v1beta1.PipelineRef{
								Name:   "java-builder",
								Bundle: defaultPipelineBundle,
							},
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
							PipelineRef: v1beta1.PipelineRef{
								Name:   "noop",
								Bundle: defaultPipelineBundle,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, selectors)).To(Succeed())

			devfile := `
                schemaVersion: 2.2.0
                metadata:
                    name: devfile-java
                    language: java
            `
			setComponentDevfile(resourceKey, devfile)

			waitOneInitialPipelineRunCreated(resourceKey)
			pipelineRun := listComponentPipelineRuns(resourceKey)[0]

			Expect(pipelineRun.Labels[ApplicationNameLabelName]).To(Equal(HASAppName))
			Expect(pipelineRun.Labels[ComponentNameLabelName]).To(Equal(HASCompName))

			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"]).To(Equal("java-builder"))
			Expect(pipelineRun.Annotations["build.appstudio.redhat.com/bundle"]).To(Equal(defaultPipelineBundle))

			Expect(pipelineRun.Spec.PipelineSpec).To(BeNil())

			Expect(pipelineRun.Spec.PipelineRef).ToNot(BeNil())
			Expect(pipelineRun.Spec.PipelineRef.Name).To(Equal("java-builder"))
			Expect(pipelineRun.Spec.PipelineRef.Bundle).To(Equal(defaultPipelineBundle))

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

})
