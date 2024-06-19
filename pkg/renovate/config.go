package renovate

import (
	"os"

	"k8s.io/utils/strings/slices"
)

const (
	RenovateMatchPatternEnvName = "RENOVATE_PATTERN"
	DefaultRenovateMatchPattern = "^quay.io/(redhat-appstudio-tekton-catalog|konflux-ci/tekton-catalog)/"
)

var (
	DisableAllPackageRules = PackageRule{MatchPackagePatterns: []string{"*"}, Enabled: false}
)

type UpdateConfig struct {
	Platform            string            `json:"platform"`
	Username            string            `json:"username"`
	GitAuthor           string            `json:"gitAuthor"`
	Onboarding          bool              `json:"onboarding"`
	RequireConfig       string            `json:"requireConfig"`
	EnabledManagers     []string          `json:"enabledManagers"`
	Repositories        []*Repository     `json:"repositories"`
	CustomManagers      []*CustomManager  `json:"customManagers,omitempty"`
	RegistryAliases     map[string]string `json:"registryAliases,omitempty"`
	PackageRules        []PackageRule     `json:"packageRules,omitempty"`
	Tekton              *Tekton           `json:"tekton,omitempty"`
	ForkProcessing      string            `json:"forkProcessing"`
	DependencyDashboard bool              `json:"dependencyDashboard"`
	Endpoint            string            `json:"endpoint,omitempty"`
}
type AuthenticatedUpdateConfig struct {
	Config UpdateConfig
	Token  string `json:"-"`
}

type Repository struct {
	Repository   string   `json:"repository"`
	BaseBranches []string `json:"baseBranches"`
}
type CustomManager struct {
	FileMatch  []string `json:"fileMatch,omitempty"`
	CustomType string   `json:"customType"`
	RegexManagerConfig
}

type RegexManagerConfig struct {
	MatchStrings []string `json:"matchStrings"`
	RegexManagerTemplates
}

type RegexManagerTemplates struct {
	DatasourceTemplate   string `json:"datasourceTemplate"`
	CurrentValueTemplate string `json:"currentValueTemplate"`
	DepNameTemplate      string `json:"depNameTemplate"`
}

func (r *Repository) AddBranch(branch string) {
	if !slices.Contains(r.BaseBranches, branch) {
		r.BaseBranches = append(r.BaseBranches, branch)
	}
}

type Tekton struct {
	FileMatch    []string      `json:"fileMatch"`
	IncludePaths []string      `json:"includePaths"`
	PackageRules []PackageRule `json:"packageRules"`
}

type PackageRule struct {
	MatchPackagePatterns []string `json:"matchPackagePatterns,omitempty"`
	MatchPackageNames    []string `json:"matchPackageNames,omitempty"`
	GroupName            string   `json:"groupName,omitempty"`
	BranchName           string   `json:"branchName,omitempty"`
	MatchDepPatterns     []string `json:"matchDepPatterns,omitempty"`
	CommitBody           string   `json:"commitBody,omitempty"`
	CommitMessageExtra   string   `json:"commitMessageExtra,omitempty"`
	CommitMessageTopic   string   `json:"commitMessageTopic,omitempty"`
	SemanticCommits      string   `json:"semanticCommits,omitempty"`
	PRFooter             string   `json:"prFooter,omitempty"`
	PRBodyColumns        []string `json:"prBodyColumns,omitempty"`
	PRBodyDefinitions    string   `json:"prBodyDefinitions,omitempty"`
	PRBodyTemplate       string   `json:"prBodyTemplate,omitempty"`
	RecreateWhen         string   `json:"recreateWhen,omitempty"`
	RebaseWhen           string   `json:"rebaseWhen,omitempty"`
	Enabled              bool     `json:"enabled"`
	FollowTag            string   `json:"followTag,omitempty"`
}

func GetRenovatePatternConfiguration() string {
	renovatePattern := os.Getenv(RenovateMatchPatternEnvName)
	if renovatePattern == "" {
		renovatePattern = DefaultRenovateMatchPattern
	}
	return renovatePattern
}
