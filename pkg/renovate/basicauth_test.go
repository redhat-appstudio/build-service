package renovate

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/konflux-ci/build-service/pkg/git"
	"github.com/konflux-ci/build-service/pkg/git/credentials"
)

var staticCredentials = &credentials.BasicAuthCredentials{Username: "usr", Password: "pwd-234"}
var StaticCredentialsFunc credentials.BasicAuthCredentialsProviderFunc = func(ctx context.Context, component *git.ScmComponent) (*credentials.BasicAuthCredentials, error) {
	return staticCredentials, nil
}

func TestNewTargets(t *testing.T) {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))
	tests := []struct {
		name            string
		credentialsFunc credentials.BasicAuthCredentialsProviderFunc
		components      []*git.ScmComponent
		expected        []*UpdateTarget
	}{
		{
			name:            "No components - no targets",
			credentialsFunc: StaticCredentialsFunc,
		},
		{
			name:            "Simple one component case",
			credentialsFunc: StaticCredentialsFunc,
			components:      []*git.ScmComponent{ignoreError(git.NewScmComponent("github", "https://github.com/umbrellacorp/devfile-sample-go-basic", "main", "devfile-sample-go-basic", "umbrellacorp-tenant")).(*git.ScmComponent)},
			expected: []*UpdateTarget{
				NewBasicAuthTask("github", "github.com", "https://api.github.com/", staticCredentials, []*Repository{
					{
						Repository:   "umbrellacorp/devfile-sample-go-basic",
						BaseBranches: []string{"main"},
					},
				}),
			},
		},
		{
			name:            "Multiple components with the same host",
			credentialsFunc: StaticCredentialsFunc,
			components: []*git.ScmComponent{
				ignoreError(
					git.NewScmComponent(
						"github",
						"https://github.com/umbrellacorp/devfile-sample-python-basic",
						"develop",
						"devfile-sample-python-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),

				ignoreError(
					git.NewScmComponent(
						"github",
						"https://github.com/umbrellacorp/devfile-sample-go-basic",
						"main",
						"devfile-sample-go-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),
			},
			expected: []*UpdateTarget{
				NewBasicAuthTask("github", "github.com", "https://api.github.com/", staticCredentials, []*Repository{
					{
						Repository:   "umbrellacorp/devfile-sample-python-basic",
						BaseBranches: []string{"develop"},
					},
					{
						Repository:   "umbrellacorp/devfile-sample-go-basic",
						BaseBranches: []string{"main"},
					},
				}),
			},
		},
		{
			name:            "Multiple components with the same host with two branches",
			credentialsFunc: StaticCredentialsFunc,
			components: []*git.ScmComponent{
				ignoreError(
					git.NewScmComponent(
						"github",
						"https://github.com/umbrellacorp/devfile-sample-python-basic",
						"develop",
						"devfile-sample-python-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),
				ignoreError(
					git.NewScmComponent(
						"github",
						"https://github.com/umbrellacorp/devfile-sample-python-basic",
						"main",
						"devfile-sample-python-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),

				ignoreError(
					git.NewScmComponent(
						"github",
						"https://github.com/umbrellacorp/devfile-sample-go-basic",
						"main",
						"devfile-sample-go-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),
			},
			expected: []*UpdateTarget{
				NewBasicAuthTask("github", "github.com", "https://api.github.com/", staticCredentials, []*Repository{
					{
						Repository:   "umbrellacorp/devfile-sample-python-basic",
						BaseBranches: []string{"develop", "main"},
					},
					{
						Repository:   "umbrellacorp/devfile-sample-go-basic",
						BaseBranches: []string{"main"},
					},
				}),
			},
		},
		{
			name:            "Multiple components with the different host",
			credentialsFunc: StaticCredentialsFunc,
			components: []*git.ScmComponent{
				ignoreError(
					git.NewScmComponent(
						"github",
						"https://github.com/umbrellacorp/devfile-sample-python-basic",
						"develop",
						"devfile-sample-python-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),

				ignoreError(
					git.NewScmComponent(
						"gitlab",
						"https://gitlab.com/umbrellacorp/devfile-sample-go-basic",
						"main",
						"devfile-sample-go-basic",
						"umbrellacorp-tenant")).(*git.ScmComponent),
			},
			expected: []*UpdateTarget{
				NewBasicAuthTask("github", "github.com", "https://api.github.com/", staticCredentials, []*Repository{
					{
						Repository:   "umbrellacorp/devfile-sample-python-basic",
						BaseBranches: []string{"develop"},
					},
				}),
				NewBasicAuthTask("gitlab", "gitlab.com", "https://gitlab.com/api/v4/", staticCredentials, []*Repository{
					{
						Repository:   "umbrellacorp/devfile-sample-go-basic",
						BaseBranches: []string{"main"},
					},
				}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//given
			TargetProvider := NewBasicAuthTargetProvider(tt.credentialsFunc)
			//when
			got := TargetProvider.GetUpdateTargets(context.TODO(), tt.components)
			//then
			sort.Slice(got, func(i, j int) bool {
				return got[i].Platform < got[j].Platform
			})
			assert.Equal(t, tt.expected, got)
		})
	}

}

func ignoreError(val interface{}, err error) interface{} {
	return val
}
