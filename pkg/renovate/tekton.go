package renovate

import "fmt"

func NewTektonUpdateConfig(platform, endpoint, username, gitAuthor string, repositories []*Repository) UpdateConfig {
	renovatePattern := GetRenovatePatternConfiguration()
	return UpdateConfig{
		Platform:        platform,
		Username:        username,
		GitAuthor:       gitAuthor,
		Onboarding:      false,
		RequireConfig:   "ignored",
		EnabledManagers: []string{"tekton"},
		Endpoint:        endpoint,
		Repositories:    repositories,
		Tekton: &Tekton{FileMatch: []string{"\\.yaml$", "\\.yml$"}, IncludePaths: []string{".tekton/**"}, PackageRules: []PackageRule{DisableAllPackageRules, {
			MatchPackagePatterns: []string{renovatePattern},
			MatchDepPatterns:     []string{renovatePattern},
			GroupName:            "Konflux references",
			BranchName:           "konflux/references/{{baseBranch}}",
			CommitMessageExtra:   "",
			CommitMessageTopic:   "Konflux references",
			CommitBody:           "Signed-off-by: {{{gitAuthor}}}",
			SemanticCommits:      "enabled",
			PRFooter:             "To execute skipped test pipelines write comment `/ok-to-test`",
			PRBodyColumns:        []string{"Package", "Change", "Notes"},
			PRBodyDefinitions:    fmt.Sprintf("{ \"Notes\": \"{{#if (or (containsString updateType 'minor') (containsString updateType 'major'))}}:warning:[migration](https://github.com/redhat-appstudio/build-definitions/blob/main/task/{{{replace '%stask-' '' packageName}}}/{{{newVersion}}}/MIGRATION.md):warning:{{/if}}\" }", renovatePattern),
			PRBodyTemplate:       "{{{header}}}{{{table}}}{{{notes}}}{{{changelogs}}}{{{footer}}}",
			RecreateWhen:         "always",
			RebaseWhen:           "behind-base-branch",
			Enabled:              true,
		}}},
		ForkProcessing:      "enabled",
		DependencyDashboard: false,
	}
}
