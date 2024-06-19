package renovate

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/konflux-ci/build-service/pkg/git"
	"github.com/konflux-ci/build-service/pkg/git/credentials"
	"github.com/konflux-ci/build-service/pkg/k8s"
)

// UpdateTarget represents a target source code repository to be executed by Renovate with credentials and repositories
type UpdateTarget struct {
	Platform     string
	Username     string
	GitAuthor    string
	Token        string
	Endpoint     string
	Repositories []*Repository
}

func (t *UpdateTarget) TektonDependencyUpdate() *AuthenticatedUpdateConfig {
	return &AuthenticatedUpdateConfig{Config: NewTektonUpdateConfig(t.Platform, t.Endpoint, t.Username, t.GitAuthor, t.Repositories), Token: t.Token}
}

func (t *UpdateTarget) NudgeDependencyUpdate(buildResult *BuildResult) *AuthenticatedUpdateConfig {
	return &AuthenticatedUpdateConfig{Config: NewNudgeDependencyUpdateConfig(buildResult, t.Platform, t.Endpoint, t.Username, t.GitAuthor, t.Repositories), Token: t.Token}
}

// AddNewBranchToTheExistedRepositoryOnTheSameHosts iterates over the targets and adds a new branch to the repository if it already exists
// NOTE: performing this operation on a slice containing targets from different platforms or hosts is unsafe.
func AddNewBranchToTheExistedRepositoryOnTheSameHosts(targets []*UpdateTarget, component *git.ScmComponent) bool {
	for _, t := range targets {
		for _, r := range t.Repositories {
			if r.Repository == component.Repository() {
				r.AddBranch(component.Branch())
				return true
			}
		}
	}
	return false
}

// AddNewRepoToTargetOnTheSameHostsWithSameCredentials iterates over the targets and adds a new repository to the target with same credentials
// NOTE: performing this operation on a slice containing targets from different platforms or hosts is unsafe.
func AddNewRepoToTargetOnTheSameHostsWithSameCredentials(targets []*UpdateTarget, component *git.ScmComponent, cred *credentials.BasicAuthCredentials) bool {
	for _, t := range targets {
		if t.Token == cred.Password && t.Username == cred.Username {
			//double check if the repository is already added
			for _, r := range t.Repositories {
				if r.Repository == component.Repository() {
					return true
				}
			}
			t.Repositories = append(t.Repositories, &Repository{
				Repository:   component.Repository(),
				BaseBranches: []string{component.Branch()},
			})
			return true
		}
	}
	return false
}

func UpdateTargetsToTektonDependeciesUpdate(targets []*UpdateTarget) []*AuthenticatedUpdateConfig {
	var configs []*AuthenticatedUpdateConfig
	for _, task := range targets {
		configs = append(configs, task.TektonDependencyUpdate())
	}
	return configs

}
func UpdateTargetToNudgeUpdate(targets []*UpdateTarget, buildResult *BuildResult) []*AuthenticatedUpdateConfig {
	var configs []*AuthenticatedUpdateConfig
	for _, task := range targets {
		configs = append(configs, task.NudgeDependencyUpdate(buildResult))
	}
	return configs

}

// UpdateTargetProvider is an interface for providing targets to be executed by Renovate
type UpdateTargetProvider interface {
	GetUpdateTargets(ctx context.Context, components []*git.ScmComponent) []*UpdateTarget
}

var _ UpdateTargetProvider = (*CompositeUpdateTargetProvider)(nil)

type CompositeUpdateTargetProvider struct {
	basicAuthTargetProvider BasicAuthTargetProvider
	githubAppTargetProvider GithubAppRenovaterTargetProvider
}

func (c CompositeUpdateTargetProvider) GetUpdateTargets(ctx context.Context, components []*git.ScmComponent) []*UpdateTarget {
	var targets []*UpdateTarget
	log := ctrllog.FromContext(ctx)
	newTargets := c.githubAppTargetProvider.GetUpdateTargets(ctx, components)
	log.Info("found new targets", "targets", len(newTargets), "provider", reflect.TypeOf(c.githubAppTargetProvider).String())
	if len(newTargets) > 0 {
		targets = append(targets, newTargets...)
	}
	newTargets = c.basicAuthTargetProvider.GetUpdateTargets(ctx, components)
	log.Info("found new targets", "targets", len(newTargets), "provider", reflect.TypeOf(c.basicAuthTargetProvider).String())
	if len(newTargets) > 0 {
		targets = append(targets, newTargets...)
	}
	return targets
}

func NewCompositeUpdateTargetProvider(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder) UpdateTargetProvider {
	return &CompositeUpdateTargetProvider{
		basicAuthTargetProvider: NewBasicAuthTargetProvider(k8s.NewGitCredentialProvider(client)),
		githubAppTargetProvider: NewGithubAppRenovaterTargetProvider(k8s.NewGithubAppConfigReader(client, scheme, eventRecorder))}
}
