/*
Copyright 2023 Red Hat, Inc.

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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/konflux-ci/build-service/pkg/common"
	"github.com/konflux-ci/build-service/pkg/git/github"
	"github.com/konflux-ci/build-service/pkg/renovate"
)

var _ = Describe("Git tekton resources renovater", func() {

	var (
		// All related to the component resources have the same key (but different type)
		pacSecretKey = types.NamespacedName{Name: PipelinesAsCodeGitHubAppSecretName, Namespace: BuildServiceNamespaceName}
	)

	Context("Test Renovate jobs creation", Label("renovater"), func() {
		var resourceTriggerKey = types.NamespacedName{Name: HASCompName + "-testtrigger", Namespace: HASAppNamespace}

		_ = BeforeEach(func() {
			createNamespace(BuildServiceNamespaceName)
			pacSecretData := map[string]string{
				"github-application-id": "12345",
				"github-private-key":    githubAppPrivateKey,
			}
			createSecret(pacSecretKey, pacSecretData)
		})

		_ = AfterEach(func() {
			deleteBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			deleteJobs(BuildServiceNamespaceName)
			os.Unsetenv(renovate.InstallationsPerJobEnvName)
			deleteSecret(pacSecretKey)
		})

		It("It should trigger job", func() {
			installedRepositoryUrls := []string{
				"https://github/test/repo1",
				"https://github/test/repo2",
			}
			github.GetAllAppInstallations = func(appIdStr string, privateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
				repositories := generateRepositories(installedRepositoryUrls)
				return []github.ApplicationInstallation{generateInstallation(repositories)}, "slug", nil
			}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourceTriggerKey,
				gitURL:       "https://github/test/repo1",
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			Eventually(listJobs).WithArguments(BuildServiceNamespaceName).WithTimeout(timeout).Should(HaveLen(1))
			deleteComponent(resourceTriggerKey)
		})

		It("It should trigger job, use default branch and default installation per job", func() {
			installedRepositoryUrls := []string{
				"https://github/test/repo1",
				"https://github/test/repo2",
			}
			github.GetAllAppInstallations = func(appIdStr string, privateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
				repositories := generateRepositories(installedRepositoryUrls)
				return []github.ApplicationInstallation{generateInstallation(repositories)}, "slug", nil
			}

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourceTriggerKey,
				gitURL:       "https://github/test/repo1",
				gitRevision:  "",
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			os.Setenv(renovate.InstallationsPerJobEnvName, "0")
			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			Eventually(listJobs).WithArguments(BuildServiceNamespaceName).WithTimeout(timeout).Should(HaveLen(1))
			deleteComponent(resourceTriggerKey)
		})

		It("It should trigger 2 jobs", func() {
			installedRepositories := make([][]string, 0, renovate.TargetsPerJob*2)
			for i := 0; i < renovate.TargetsPerJob*2; i++ {
				installedRepositories = append(installedRepositories, []string{
					fmt.Sprintf("https://github/test%v/repo%v", i, 1),
					fmt.Sprintf("https://github/test%v/repo%v", i, 2),
					fmt.Sprintf("https://github/test%v/repo%v", i, 3),
				})

			}
			var gitApplication []github.ApplicationInstallation
			for _, installedRepositoryUrls := range installedRepositories {
				gitApplication = append(gitApplication, generateInstallation(generateRepositories(installedRepositoryUrls)))
			}

			github.GetAllAppInstallations = func(appIdStr string, privateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
				return gitApplication, "slug", nil
			}

			var componentsNs []types.NamespacedName
			var resourceKey types.NamespacedName
			for i, installedRepositoryUrls := range installedRepositories {
				for j, installedRepositoryUrl := range installedRepositoryUrls {
					resourceKey = types.NamespacedName{Name: HASCompName + "-testtrigger" + fmt.Sprintf("test%v-%v", i, j), Namespace: HASAppNamespace}
					createCustomComponentWithBuildRequest(componentConfig{
						componentKey: resourceKey,
						gitURL:       installedRepositoryUrl,
						annotations:  defaultPipelineAnnotations,
					}, BuildRequestConfigurePaCAnnotationValue)
					componentsNs = append(componentsNs, resourceKey)
				}
			}
			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			Eventually(listJobs).WithArguments(BuildServiceNamespaceName).WithTimeout(timeout).Should(HaveLen(2))
			//now delete all components
			for _, componentNs := range componentsNs {
				deleteComponent(componentNs)
			}
		})

		It("It should trigger job, because pac secret is missing", func() {
			installedRepositoryUrls := []string{
				"https://github/test/repo1",
				"https://github/test/repo2",
			}
			github.GetAllAppInstallations = func(appIdStr string, privateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
				repositories := generateRepositories(installedRepositoryUrls)
				return []github.ApplicationInstallation{generateInstallation(repositories)}, "slug", nil
			}

			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourceTriggerKey,
				gitURL:       "https://github/test/repo1",
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			deleteSecret(pacSecretKey)
			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			Eventually(listEvents).WithArguments("default").WithTimeout(timeout).ShouldNot(BeEmpty())
			allEvents := listEvents("default")
			renovaterReason := ""
			// find event only for renovater, because PaC will throw event as well which is PaCSecretNotFound
			for _, event := range allEvents {
				fmt.Println(event.Reason)
				fmt.Println(event.ReportingController)
				if event.ReportingController == "GitTektonResourcesRenovater" {
					renovaterReason = event.Reason
					break
				}
			}
			Expect(renovaterReason).To(Equal("ErrorReadingPaCSecret"))
			deleteComponent(resourceTriggerKey)
		})

		It("It should not trigger job", func() {
			installedRepositoryUrls := []string{
				"https://github/test/repo1",
				"https://github/test/repo2",
			}
			github.GetAllAppInstallations = func(appIdStr string, privateKeyPem []byte) ([]github.ApplicationInstallation, string, error) {
				repositories := generateRepositories(installedRepositoryUrls)
				return []github.ApplicationInstallation{generateInstallation(repositories)}, "slug", nil
			}
			createCustomComponentWithBuildRequest(componentConfig{
				componentKey: resourceTriggerKey,
				gitURL:       "https://github/test/repo3",
				annotations:  defaultPipelineAnnotations,
			}, BuildRequestConfigurePaCAnnotationValue)

			createDefaultBuildPipelineConfigMap(defaultPipelineConfigMapKey)
			Consistently(listJobs).WithArguments(BuildServiceNamespaceName).WithTimeout(time.Second * 5).Should(BeEmpty())

			deleteComponent(resourceTriggerKey)
		})
	})
})
