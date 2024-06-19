package renovate

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/konflux-ci/build-service/pkg/git"
	l "github.com/konflux-ci/build-service/pkg/logs"
)

const DefaultNudgeFiles = ".*Dockerfile.*, .*.yaml, .*Containerfile.*"

type BuildResult struct {
	BuiltImageRepository      string
	BuiltImageTag             string
	Digest                    string
	DistributionRepositories  []string
	FileMatches               string
	UpdatedComponentName      string
	UpdatedComponentNamespace string
}

func (b *BuildResult) SeparatedFileMatches() []string {
	fileMatchParts := strings.Split(b.FileMatches, ",")
	for i := range fileMatchParts {
		fileMatchParts[i] = strings.TrimSpace(fileMatchParts[i])
	}
	return fileMatchParts
}

type ComponentDependenciesUpdater interface {
	Update(ctx context.Context, components []*git.ScmComponent, buildResult *BuildResult) error
}

type DefaultComponentDependenciesUpdater struct {
	updateCoordinator    *UpdateCoordinator
	updateTargetProvider UpdateTargetProvider
}

func NewDefaultComponentDependenciesUpdater(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder) *DefaultComponentDependenciesUpdater {
	return &DefaultComponentDependenciesUpdater{updateCoordinator: NewUpdateCoordinator(client, scheme), updateTargetProvider: NewCompositeUpdateTargetProvider(client, scheme, eventRecorder)}
}

func (d DefaultComponentDependenciesUpdater) Update(ctx context.Context, components []*git.ScmComponent, buildResult *BuildResult) error {
	log := ctrllog.FromContext(ctx)
	updateTargets := d.updateTargetProvider.GetUpdateTargets(ctx, components)
	log.V(l.DebugLevel).Info("found renovate targets", "targets", len(updateTargets))
	if err := d.updateCoordinator.ExecutePipelineRun(ctx, buildResult.UpdatedComponentNamespace, UpdateTargetToNudgeUpdate(updateTargets, buildResult)); err != nil {
		return err
	}
	return nil
}

func NewNudgeDependencyUpdateConfig(buildResult *BuildResult, platform, endpoint, username, gitAuthor string, repositories []*Repository) UpdateConfig {

	var matchStrings []string
	var matchPackageNames []string
	var registryAliases = make(map[string]string)
	var packageRules []PackageRule
	packageRules = append(packageRules, DisableAllPackageRules)
	matchStrings = append(matchStrings, buildResult.BuiltImageRepository+"(:.*)?@(?<currentDigest>sha256:[a-f0-9]+)")
	packageRules = append(packageRules, PackageRule{
		MatchPackageNames:  append(append(matchPackageNames, buildResult.BuiltImageRepository), buildResult.DistributionRepositories...),
		GroupName:          "Component Update " + buildResult.UpdatedComponentName,
		BranchName:         "rhtap/component-updates/" + buildResult.UpdatedComponentName,
		CommitMessageTopic: buildResult.UpdatedComponentName,
		PRFooter:           "To execute skipped test pipelines write comment `/ok-to-test`",
		RecreateWhen:       "always",
		RebaseWhen:         "behind-base-branch",
		Enabled:            true,
		FollowTag:          buildResult.BuiltImageTag,
	})
	for _, drepositiry := range buildResult.DistributionRepositories {
		matchStrings = append(matchStrings, drepositiry+"(:.*)?@(?<currentDigest>sha256:[a-f0-9]+)")
		registryAliases[drepositiry] = buildResult.BuiltImageRepository

	}
	return UpdateConfig{
		Platform:        platform,
		Username:        username,
		GitAuthor:       gitAuthor,
		Onboarding:      false,
		RequireConfig:   "ignored",
		EnabledManagers: []string{"regex"},
		CustomManagers: []*CustomManager{
			{
				FileMatch:  buildResult.SeparatedFileMatches(),
				CustomType: "regex",
				RegexManagerConfig: RegexManagerConfig{
					MatchStrings: matchStrings,
					RegexManagerTemplates: RegexManagerTemplates{
						DatasourceTemplate:   "docker",
						CurrentValueTemplate: buildResult.BuiltImageTag,
						DepNameTemplate:      buildResult.BuiltImageRepository,
					},
				},
			},
		},
		Endpoint:            endpoint,
		Repositories:        repositories,
		ForkProcessing:      "enabled",
		DependencyDashboard: false,
		RegistryAliases:     registryAliases,
		PackageRules:        packageRules,
	}
}
