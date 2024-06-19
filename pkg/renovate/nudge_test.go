package renovate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNudgeDependencyUpdateConfig(t *testing.T) {
	type args struct {
		buildResult  *BuildResult
		platform     string
		endpoint     string
		username     string
		gitAuthor    string
		repositories []*Repository
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Test NewNudgeDependencyUpdateConfig",
			args: args{
				platform:  "github",
				username:  "slug1[bot]",
				gitAuthor: "slug1 <123456+slug1[bot]@users.noreply.github.com>",
				repositories: []*Repository{
					{
						Repository:   "repo1",
						BaseBranches: []string{"main"},
					},
				},
				buildResult: &BuildResult{
					BuiltImageRepository:     "quay.io/sdouglas/multi-component-parent-image",
					BuiltImageTag:            "a8dce08dbdf290e5d616a83672ad3afcb4b455ef",
					Digest:                   "sha256:716be32f12f0dd31adbab1f57e9d0b87066e03de51c89dce8ffb397fbac92314",
					DistributionRepositories: []string{"registry.redhat.com/some-product", "registry.redhat.com/other-product"},
					FileMatches:              DefaultNudgeFiles,
					UpdatedComponentName:     "test-component",
				},
			},
			want: `{
   "platform": "github",
   "username": "slug1[bot]",
   "gitAuthor": "slug1 \u003c123456+slug1[bot]@users.noreply.github.com\u003e",
   "onboarding": false,
   "requireConfig": "ignored",
   "enabledManagers": [
      "regex"
   ],
   "repositories": [
      {
         "repository": "repo1",
         "baseBranches": [
            "main"
         ]
      }
   ],
   "customManagers": [
      {
         "fileMatch": [
            ".*Dockerfile.*",
            ".*.yaml",
            ".*Containerfile.*"
         ],
         "customType": "regex",
         "matchStrings": [
            "quay.io/sdouglas/multi-component-parent-image(:.*)?@(?\u003ccurrentDigest\u003esha256:[a-f0-9]+)",
            "registry.redhat.com/some-product(:.*)?@(?\u003ccurrentDigest\u003esha256:[a-f0-9]+)",
            "registry.redhat.com/other-product(:.*)?@(?\u003ccurrentDigest\u003esha256:[a-f0-9]+)"
         ],
         "datasourceTemplate": "docker",
         "currentValueTemplate": "a8dce08dbdf290e5d616a83672ad3afcb4b455ef",
         "depNameTemplate": "quay.io/sdouglas/multi-component-parent-image"
      }
   ],
   "registryAliases": {
      "registry.redhat.com/other-product": "quay.io/sdouglas/multi-component-parent-image",
      "registry.redhat.com/some-product": "quay.io/sdouglas/multi-component-parent-image"
   },
   "packageRules": [
      {
         "matchPackagePatterns": [
            "*"
         ],
         "enabled": false
      },
      {
         "matchPackageNames": [
            "quay.io/sdouglas/multi-component-parent-image",
            "registry.redhat.com/some-product",
            "registry.redhat.com/other-product"
         ],
         "groupName": "Component Update test-component",
         "branchName": "rhtap/component-updates/test-component",
         "commitMessageTopic": "test-component",
         "prFooter": "To execute skipped test pipelines write comment ` + "`/ok-to-test`" + `",
         "recreateWhen": "always",
         "rebaseWhen": "behind-base-branch",
         "enabled": true,
         "followTag": "a8dce08dbdf290e5d616a83672ad3afcb4b455ef"
      }
   ],
   "forkProcessing": "enabled",
   "dependencyDashboard": false
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := NewNudgeDependencyUpdateConfig(tt.args.buildResult, tt.args.platform, tt.args.endpoint, tt.args.username, tt.args.gitAuthor, tt.args.repositories)
			//json, err := json.Marshal(update)
			json, err := json.MarshalIndent(update, "", "   ")
			if err != nil {
				t.Errorf("Error marshalling json: %v", err)
			}
			assert.Equalf(t, tt.want, string(json), "NewNudgeDependencyUpdateConfig(%v, %v, %v, %v, %v, %v)", tt.args.buildResult, tt.args.platform, tt.args.endpoint, tt.args.username, tt.args.gitAuthor, tt.args.repositories)
		})
	}
}
