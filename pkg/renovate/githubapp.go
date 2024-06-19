package renovate

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/konflux-ci/build-service/pkg/git"
	"github.com/konflux-ci/build-service/pkg/git/github"
	"github.com/konflux-ci/build-service/pkg/git/githubapp"
)

// GithubAppRenovaterTargetProvider is an implementation of UpdateTargetProvider that provides Renovate targets for GitHub App installations.
type GithubAppRenovaterTargetProvider struct {
	appConfigReader githubapp.ConfigReader
}

func NewGithubAppRenovaterTargetProvider(appConfigReader githubapp.ConfigReader) GithubAppRenovaterTargetProvider {
	return GithubAppRenovaterTargetProvider{appConfigReader: appConfigReader}
}
func (g GithubAppRenovaterTargetProvider) GetUpdateTargets(ctx context.Context, components []*git.ScmComponent) []*UpdateTarget {
	log := ctrllog.FromContext(ctx)
	githubAppId, privateKey, err := g.appConfigReader.GetConfig(ctx)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get GitHub App configuration")
		}
		return nil
	}
	githubAppInstallations, slug, err := github.GetAllAppInstallations(githubAppId, privateKey)
	if err != nil {
		log.Error(err, "failed to get GitHub App installations")
		return nil
	}
	componentUrlToBranchesMap := git.ComponentUrlToBranchesMap(components)

	// Match installed repositories with Components and get custom branch if defined
	var newTargets []*UpdateTarget
	for _, githubAppInstallation := range githubAppInstallations {
		var repositories []*Repository
		for _, repository := range githubAppInstallation.Repositories {
			branches, ok := componentUrlToBranchesMap[repository.GetHTMLURL()]
			// Filter repositories with installed GH App but missing Component
			if !ok {
				continue
			}
			for i := range branches {
				if branches[i] == git.InternalDefaultBranch {
					branches[i] = repository.GetDefaultBranch()
				}
			}

			repositories = append(repositories, &Repository{
				BaseBranches: branches,
				Repository:   repository.GetFullName(),
			})
		}
		// Do not add installation which has no matching repositories
		if len(repositories) == 0 {
			continue
		}
		newTargets = append(newTargets, newGithubTask(slug, githubAppInstallation.Token, repositories))
	}
	return newTargets
}

func newGithubTask(slug string, token string, repositories []*Repository) *UpdateTarget {
	return &UpdateTarget{
		Platform:     "github",
		Endpoint:     git.BuildAPIEndpoint("github").APIEndpoint("github.com"),
		Username:     fmt.Sprintf("%s[bot]", slug),
		GitAuthor:    fmt.Sprintf("%s <123456+%s[bot]@users.noreply.github.com>", slug, slug),
		Token:        token,
		Repositories: repositories,
	}
}
