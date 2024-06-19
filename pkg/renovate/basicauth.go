package renovate

import (
	"context"
	"fmt"

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/konflux-ci/build-service/pkg/boerrors"
	"github.com/konflux-ci/build-service/pkg/git"
	"github.com/konflux-ci/build-service/pkg/git/credentials"
)

// BasicAuthTargetProvider is an implementation of the renovate.UpdateTargetProvider that creates the renovate.UpdateTarget for the components
// based on the generic algorithm and not tied to any specific SCM provider implementation.
type BasicAuthTargetProvider struct {
	credentialsProvider credentials.BasicAuthCredentialsProvider
}

func NewBasicAuthTargetProvider(credentialsProvider credentials.BasicAuthCredentialsProvider) BasicAuthTargetProvider {
	return BasicAuthTargetProvider{
		credentialsProvider: credentialsProvider,
	}
}

// GetUpdateTargets returns the list of new renovate update targets for the components. It uses such an algorithm:
// 1. Group components by namespace
// 2. Group components by platform
// 3. Group components by host
// 4. For each host creating targetsOnHost
// 5. For each component looking for an existing target with the same repository and adding a new branch to it
// 6. If there is no target with the same repository, looking for a target with the same credentials and adding a new repository to it
// 7. If there is no target with the same credentials, creating a new target and adding it to the targetsOnHost
// 8. Adding targetsOnHost to the newTargets
func (g BasicAuthTargetProvider) GetUpdateTargets(ctx context.Context, components []*git.ScmComponent) []*UpdateTarget {
	log := ctrllog.FromContext(ctx)
	// Step 1
	componentNamespaceMap := git.NamespaceToComponentMap(components)
	log.Info("generating new renovate task in user's namespace for components", "count", len(components))
	var newTargets []*UpdateTarget
	for namespace, componentsInNamespace := range componentNamespaceMap {
		log.Info("found components", "namespace", namespace, "count", len(componentsInNamespace))
		// Step 2
		platformToComponentMap := git.PlatformToComponentMap(componentsInNamespace)
		log.Info("found git platform on namespace", "namespace", namespace, "count", len(platformToComponentMap))
		for platform, componentsOnPlatform := range platformToComponentMap {
			log.Info("processing components on platform", "platform", platform, "count", len(componentsOnPlatform))
			// Step 3
			hostToComponentsMap := git.HostToComponentMap(componentsOnPlatform)
			log.Info("found hosts on platform", "namespace", namespace, "platform", platform, "count", len(hostToComponentsMap))
			// Step 4
			var targetsOnHost []*UpdateTarget
			for host, componentsOnHost := range hostToComponentsMap {
				endpoint := git.BuildAPIEndpoint(platform).APIEndpoint(host)
				log.Info("processing components on host", "namespace", namespace, "platform", platform, "host", host, "endpoint", endpoint, "count", len(componentsOnHost))
				for _, component := range componentsOnHost {
					// Step 5
					if !AddNewBranchToTheExistedRepositoryOnTheSameHosts(targetsOnHost, component) {
						creds, err := g.credentialsProvider.GetBasicAuthCredentials(ctx, component)
						if err != nil {
							if !boerrors.IsBuildOpError(err, boerrors.EComponentGitSecretMissing) {
								log.Error(err, "failed to get basic auth credentials for component", "component", component)
							}
							continue
						}
						// Step 6
						if !AddNewRepoToTargetOnTheSameHostsWithSameCredentials(targetsOnHost, component, creds) {
							// Step 7
							targetsOnHost = append(targetsOnHost, NewBasicAuthTask(platform, host, endpoint, creds, []*Repository{
								{
									Repository:   component.Repository(),
									BaseBranches: []string{component.Branch()},
								},
							}))
						}
					}
				}
			}
			//Step 8
			newTargets = append(newTargets, targetsOnHost...)
		}

	}
	log.Info("generated new renovate targets", "count", len(newTargets))
	return newTargets
}

func NewBasicAuthTask(platform, host, endpoint string, credentials *credentials.BasicAuthCredentials, repositories []*Repository) *UpdateTarget {
	return &UpdateTarget{
		Platform:     platform,
		Username:     credentials.Username,
		GitAuthor:    fmt.Sprintf("%s <123456+%s[bot]@users.noreply.%s>", credentials.Username, credentials.Username, host),
		Token:        credentials.Password,
		Endpoint:     endpoint,
		Repositories: repositories,
	}
}
