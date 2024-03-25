/*
Copyright 2022-2023 Red Hat, Inc.

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

package github

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"

	gp "github.com/redhat-appstudio/build-service/pkg/git/gitprovider"
)

// Allow mocking for tests
var NewGithubClient func(accessToken string) *GithubClient = newGithubClient
var NewGithubClientWithBasicAuth func(username, password string) *GithubClient = newGithubClientWithBasicAuth

const (
	// Allowed values are 'json' and 'form' according to the doc: https://docs.github.com/en/rest/webhooks/repos#create-a-repository-webhook
	webhookContentType = "json"
)

var (
	appStudioPaCWebhookEvents = [...]string{"pull_request", "push", "issue_comment", "commit_comment"}
)

var _ gp.GitProviderClient = (*GithubClient)(nil)

type GithubClient struct {
	ctx    context.Context
	client *github.Client

	appId            int64
	appPrivateKeyPem []byte
}

// EnsurePaCMergeRequest creates or updates existing Pipelines as Code configuration proposal merge request
func (g *GithubClient) EnsurePaCMergeRequest(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	// Fallback to the default branch if base branch is not set
	if d.BaseBranchName == "" {
		baseBranch, err := g.getDefaultBranch(owner, repository)
		if err != nil {
			return "", err
		}
		d.BaseBranchName = baseBranch
	}

	// Check if Pipelines as Code configuration up to date in the main branch
	upToDate, err := g.filesUpToDate(owner, repository, d.BaseBranchName, d.Files)
	if err != nil {
		return "", err
	}
	if upToDate {
		// Nothing to do, the configuration is alredy in the main branch of the repository
		return "", nil
	}

	// Check if branch with a proposal exists
	branchExists, err := g.branchExist(owner, repository, d.BranchName)
	if err != nil {
		return "", err
	}

	if branchExists {
		upToDate, err := g.filesUpToDate(owner, repository, d.BranchName, d.Files)
		if err != nil {
			return "", err
		}
		if !upToDate {
			// Update branch
			branchRef, err := g.getBranch(owner, repository, d.BranchName)
			if err != nil {
				return "", err
			}

			err = g.addCommitToBranch(owner, repository, d.AuthorName, d.AuthorEmail, d.CommitMessage, d.Files, branchRef)
			if err != nil {
				return "", err
			}
		}

		pr, err := g.findPullRequestByBranchesWithinRepository(owner, repository, d.BranchName, d.BaseBranchName)
		if err != nil {
			return "", err
		}
		if pr != nil {
			return *pr.HTMLURL, nil
		}

		prUrl, err := g.createPullRequestWithinRepository(owner, repository, d.BranchName, d.BaseBranchName, d.Title, d.Text)
		if err != nil {
			if strings.Contains(err.Error(), "No commits between") {
				// This could happen when a PR was created and merged, but PR branch was not deleted. Then main was updated.
				// Current branch has correct configuration, but it's not possible to create a PR,
				// because current branch reference is included into main branch.
				if _, err := g.deleteBranch(owner, repository, d.BranchName); err != nil {
					return "", err
				}
				return g.EnsurePaCMergeRequest(repoUrl, d)
			}
		}
		return prUrl, nil

	} else {
		// Create branch, commit and pull request
		branchRef, err := g.createBranch(owner, repository, d.BranchName, d.BaseBranchName)
		if err != nil {
			return "", err
		}

		err = g.addCommitToBranch(owner, repository, d.AuthorName, d.AuthorEmail, d.CommitMessage, d.Files, branchRef)
		if err != nil {
			return "", err
		}

		return g.createPullRequestWithinRepository(owner, repository, d.BranchName, d.BaseBranchName, d.Title, d.Text)
	}
}

// UndoPaCMergeRequest creates or updates existing Pipelines as Code configuration removal merge request
func (g *GithubClient) UndoPaCMergeRequest(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	// Fallback to the default branch if base branch is not set
	if d.BaseBranchName == "" {
		baseBranch, err := g.getDefaultBranch(owner, repository)
		if err != nil {
			return "", err
		}
		d.BaseBranchName = baseBranch
	}

	files, err := g.filesExistInDirectory(owner, repository, d.BaseBranchName, ".tekton", d.Files)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		// Nothing to prune
		return "", nil
	}

	// Need to create PR that deletes PaC configuration of the component

	// Delete old branch, if any
	if _, err := g.deleteBranch(owner, repository, d.BranchName); err != nil {
		return "", err
	}

	// Create branch, commit and pull request
	branchRef, err := g.createBranch(owner, repository, d.BranchName, d.BaseBranchName)
	if err != nil {
		return "", err
	}

	err = g.addDeleteCommitToBranch(owner, repository, d.AuthorName, d.AuthorEmail, d.CommitMessage, d.Files, branchRef)
	if err != nil {
		return "", err
	}

	return g.createPullRequestWithinRepository(owner, repository, d.BranchName, d.BaseBranchName, d.Title, d.Text)
}

// FindUnmergedPaCMergeRequest finds out the unmerged merge request that is opened during the component onboarding
// An onboarding merge request fulfills both:
// 1) opened based on the base branch which is determined by the Revision or is the default branch of component repository
// 2) opened from head ref: owner:appstudio-{component.Name}
// If no onboarding merge request is found, nil is returned.
func (g *GithubClient) FindUnmergedPaCMergeRequest(repoUrl string, d *gp.MergeRequestData) (*gp.MergeRequest, error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	opts := &github.PullRequestListOptions{
		Head: fmt.Sprintf("%s:%s", owner, d.BranchName),
		Base: d.BaseBranchName,
		// Opened pull request is searched by default by GitHub API.
	}
	pullRequests, resp, err := g.client.PullRequests.List(context.Background(), owner, repository, opts)
	if err != nil {
		return nil, refineGitHostingServiceError(resp.Response, err)
	}
	if len(pullRequests) == 0 {
		return nil, nil
	}
	pr := pullRequests[0]
	return &gp.MergeRequest{
		Id:        *pr.ID,
		CreatedAt: pr.CreatedAt,
		WebUrl:    *pr.URL,
		Title:     *pr.Title,
	}, nil
}

// SetupPaCWebhook creates Pipelines as Code webhook in the given repository
func (g *GithubClient) SetupPaCWebhook(repoUrl, webhookUrl, webhookSecret string) error {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	existingWebhook, err := g.getWebhookByTargetUrl(owner, repository, webhookUrl)
	if err != nil {
		return err
	}

	defaultWebhook := getDefaultWebhookConfig(webhookUrl, webhookSecret)

	if existingWebhook == nil {
		// Webhook does not exist
		_, err = g.createWebhook(owner, repository, defaultWebhook)
		return err
	}

	// Webhook exists
	// Need to always update the webhook in order to make sure that the webhook secret is up to date
	// (it is not possible to read existing webhook secret)
	existingWebhook.Config["secret"] = webhookSecret
	// It doesn't make sense to check target URL as it is used as webhook ID
	if existingWebhook.Config["content_type"] != webhookContentType {
		existingWebhook.Config["content_type"] = webhookContentType
	}
	if existingWebhook.Config["insecure_ssl"] != "1" {
		existingWebhook.Config["insecure_ssl"] = "1"
	}

	for _, requiredWebhookEvent := range appStudioPaCWebhookEvents {
		requiredEventFound := false
		for _, existingWebhookEvent := range existingWebhook.Events {
			if existingWebhookEvent == requiredWebhookEvent {
				requiredEventFound = true
				break
			}
		}
		if !requiredEventFound {
			existingWebhook.Events = append(existingWebhook.Events, requiredWebhookEvent)
		}
	}

	if *existingWebhook.Active != *defaultWebhook.Active {
		existingWebhook.Active = defaultWebhook.Active
	}

	_, err = g.updateWebhook(owner, repository, existingWebhook)
	return err
}

func getDefaultWebhookConfig(webhookUrl, webhookSecret string) *github.Hook {
	insecureSSL := "0"
	if gp.IsInsecureSSL() {
		insecureSSL = "1"
	}
	return &github.Hook{
		Events: appStudioPaCWebhookEvents[:],
		Config: map[string]interface{}{
			"url":          webhookUrl,
			"content_type": webhookContentType,
			"secret":       webhookSecret,
			"insecure_ssl": insecureSSL,
		},
		Active: github.Bool(true),
	}
}

// DeletePaCWebhook deletes Pipelines as Code webhook in the given repository
func (g *GithubClient) DeletePaCWebhook(repoUrl, webhookUrl string) error {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	existingWebhook, err := g.getWebhookByTargetUrl(owner, repository, webhookUrl)
	if err != nil {
		return err
	}
	if existingWebhook == nil {
		// Webhook doesn't exist, nothing to do
		return nil
	}

	return g.deleteWebhook(owner, repository, *existingWebhook.ID)
}

// GetDefaultBranch returns name of default branch in the given repository
func (g *GithubClient) GetDefaultBranch(repoUrl string) (string, error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)
	return g.getDefaultBranch(owner, repository)
}

// DeleteBranch deletes given branch from repository
func (g *GithubClient) DeleteBranch(repoUrl, branchName string) (bool, error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)
	return g.deleteBranch(owner, repository, branchName)
}

// GetBranchSha returns SHA of top commit in the given branch
// If branch name is empty, default branch is used.
func (g *GithubClient) GetBranchSha(repoUrl, branchName string) (string, error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	// If branch is not specified, use default branch
	if branchName == "" {
		defaultBranchName, err := g.getDefaultBranch(owner, repository)
		if err != nil {
			return "", err
		}
		branchName = defaultBranchName
	}

	ref, err := g.getBranch(owner, repository, branchName)
	if err != nil {
		return "", err
	}
	if ref.GetObject() == nil {
		return "", fmt.Errorf("unexpected response while getting branch top commit SHA")
	}
	sha := ref.GetObject().GetSHA()
	return sha, nil
}

// IsFileExist check whether given file exists in the given branch of the reposiotry.
// If branch is empty string, default branch is used.
func (g *GithubClient) IsFileExist(repoUrl, branchName, filePath string) (bool, error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	if branchName == "" {
		var err error
		branchName, err = g.getDefaultBranch(owner, repository)
		if err != nil {
			return false, err
		}
	}

	directory := filepath.Dir(filePath)
	files, err := g.filesExistInDirectory(owner, repository, branchName, directory, []gp.RepositoryFile{{FullPath: filePath}})
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// IsRepositoryPublic returns true if the repository could be accessed without authentication
func (g *GithubClient) IsRepositoryPublic(repoUrl string) (bool, error) {
	owner, repository := getOwnerAndRepoFromUrl(repoUrl)

	repoInfo, err := g.getRepositoryInfo(owner, repository)
	if err != nil {
		return false, err
	}
	if repoInfo == nil {
		// There is no difference between private and not found for GitHub unless the user is the owner.
		return false, nil
	}
	return !*repoInfo.Private, nil
}

func (g *GithubClient) GetBrowseRepositoryAtShaLink(repoUrl, sha string) string {
	repoUrl = strings.TrimSuffix(repoUrl, ".git")
	gitSourceUrlParts := strings.Split(repoUrl, "/")
	gitProviderHost := "https://" + gitSourceUrlParts[2]
	owner := gitSourceUrlParts[3]
	repository := gitSourceUrlParts[4]

	return fmt.Sprintf("%s/%s/%s?rev=%s", gitProviderHost, owner, repository, sha)
}

func newGithubClient(accessToken string) *GithubClient {
	gh := &GithubClient{}
	gh.ctx = context.TODO()

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(gh.ctx, ts)

	gh.client = github.NewClient(tc)

	return gh
}

func newGithubClientWithBasicAuth(username, password string) *GithubClient {
	gh := &GithubClient{}
	gh.ctx = context.TODO()

	tp := github.BasicAuthTransport{
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
	}
	gh.client = github.NewClient(tp.Client())

	return gh
}
