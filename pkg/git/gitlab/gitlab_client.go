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

package gitlab

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xanzy/go-gitlab"

	gp "github.com/redhat-appstudio/build-service/pkg/git/gitprovider"
)

// Allow mocking for tests
var NewGitlabClient func(accessToken, baseUrl string) (*GitlabClient, error) = newGitlabClient
var NewGitlabClientWithBasicAuth func(username, password, baseUrl string) (*GitlabClient, error) = newGitlabClientWithBasicAuth

var _ gp.GitProviderClient = (*GitlabClient)(nil)

type GitlabClient struct {
	client *gitlab.Client
}

// EnsurePaCMergeRequest creates or updates existing (if needed) Pipelines as Code configuration proposal merge request.
// Returns the merge request web URL.
// If there is no error and web URL is empty, it means that the merge request is not needed (main branch is up to date).
func (g *GitlabClient) EnsurePaCMergeRequest(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return "", err
	}

	// Fallback to the default branch if base branch is not set
	if d.BaseBranchName == "" {
		baseBranch, err := g.getDefaultBranch(projectPath)
		if err != nil {
			return "", err
		}
		d.BaseBranchName = baseBranch
	}

	pacConfigurationUpToDate, err := g.filesUpToDate(projectPath, d.BaseBranchName, d.Files)
	if err != nil {
		return "", err
	}
	if pacConfigurationUpToDate {
		// Nothing to do, the configuration is alredy in the main branch of the repository
		return "", nil
	}

	mrBranchExists, err := g.branchExist(projectPath, d.BranchName)
	if err != nil {
		return "", err
	}

	if mrBranchExists {
		mrBranchUpToDate, err := g.filesUpToDate(projectPath, d.BranchName, d.Files)
		if err != nil {
			return "", err
		}
		if !mrBranchUpToDate {
			err := g.commitFilesIntoBranch(projectPath, d.BranchName, d.CommitMessage, d.AuthorName, d.AuthorEmail, d.Files)
			if err != nil {
				return "", err
			}
		}

		mr, err := g.findMergeRequestByBranches(projectPath, d.BranchName, d.BaseBranchName)
		if err != nil {
			return "", err
		}
		if mr != nil {
			// Merge request already exists
			return mr.WebURL, nil
		}

		diffExists, err := g.diffNotEmpty(projectPath, d.BranchName, d.BaseBranchName)
		if err != nil {
			return "", err
		}
		if !diffExists {
			// This situation occurs if an MR was merged but the branch was not deleted and main is changed after the merge.
			// Despite the fact that there is actual diff between branches, git treats it as no diff,
			// because the branch is already "included" in main.
			if _, err := g.deleteBranch(projectPath, d.BranchName); err != nil {
				return "", err
			}
			return g.EnsurePaCMergeRequest(repoUrl, d)
		}

		return g.createMergeRequestWithinRepository(projectPath, d.BranchName, d.BaseBranchName, d.Title, d.Text)
	} else {
		// Need to create branch and MR with Pipelines as Code configuration
		err = g.createBranch(projectPath, d.BranchName, d.BaseBranchName)
		if err != nil {
			return "", err
		}

		err = g.commitFilesIntoBranch(projectPath, d.BranchName, d.CommitMessage, d.AuthorName, d.AuthorEmail, d.Files)
		if err != nil {
			return "", err
		}

		return g.createMergeRequestWithinRepository(projectPath, d.BranchName, d.BaseBranchName, d.Title, d.Text)
	}
}

// UndoPaCMergeRequest creates or updates existing Pipelines as Code configuration removal merge request.
// Returns the merge request web URL.
// If there is no error and web URL is empty, it means that the merge request is not needed (the configuraton has already been deleted).
func (g *GitlabClient) UndoPaCMergeRequest(repoUrl string, d *gp.MergeRequestData) (webUrl string, err error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return "", err
	}

	// Fallback to the default branch if base branch is not set
	if d.BaseBranchName == "" {
		baseBranchName, err := g.getDefaultBranch(projectPath)
		if err != nil {
			return "", err
		}
		d.BaseBranchName = baseBranchName
	}

	files, err := g.filesExistInDirectory(projectPath, d.BaseBranchName, ".tekton", d.Files)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		// Nothing to prune
		return "", nil
	}

	// Need to create MR that deletes PaC configuration of the component

	// Delete old branch, if any
	if _, err := g.deleteBranch(projectPath, d.BranchName); err != nil {
		return "", err
	}

	// Create branch, commit and pull request
	if err := g.createBranch(projectPath, d.BranchName, d.BaseBranchName); err != nil {
		return "", err
	}

	err = g.addDeleteCommitToBranch(projectPath, d.BranchName, d.AuthorName, d.AuthorEmail, d.CommitMessage, files)
	if err != nil {
		return "", err
	}

	return g.createMergeRequestWithinRepository(projectPath, d.BranchName, d.BaseBranchName, d.Title, d.Text)
}

// FindUnmergedPaCMergeRequest searches for existing Pipelines as Code configuration proposal merge request
func (g *GitlabClient) FindUnmergedPaCMergeRequest(repoUrl string, d *gp.MergeRequestData) (*gp.MergeRequest, error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return nil, err
	}

	opts := &gitlab.ListProjectMergeRequestsOptions{
		State:          gitlab.String("opened"),
		AuthorUsername: gitlab.String(d.AuthorName),
		SourceBranch:   gitlab.String(d.BaseBranchName),
		TargetBranch:   gitlab.String(d.BranchName),
	}
	mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(projectPath, opts)
	if err != nil {
		return nil, refineGitHostingServiceError(resp.Response, err)
	}
	if len(mrs) == 0 {
		return nil, nil
	}
	mr := mrs[0]
	return &gp.MergeRequest{
		Id:        int64(mr.ID),
		CreatedAt: mr.CreatedAt,
		WebUrl:    mr.WebURL,
		Title:     mr.Title,
	}, nil
}

// SetupPaCWebhook creates Pipelines as Code webhook in the given repository
func (g *GitlabClient) SetupPaCWebhook(repoUrl, webhookUrl, webhookSecret string) error {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return err
	}

	existingWebhook, err := g.getWebhookByTargetUrl(projectPath, webhookUrl)
	if err != nil {
		return err
	}

	if existingWebhook == nil {
		_, err = g.createPaCWebhook(projectPath, webhookUrl, webhookSecret)
		return err
	}

	_, err = g.updatePaCWebhook(projectPath, existingWebhook.ID, webhookUrl, webhookSecret)
	return err
}

// DeletePaCWebhook deletes Pipelines as Code webhook in the given repository
func (g *GitlabClient) DeletePaCWebhook(repoUrl, webhookUrl string) error {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return err
	}

	existingWebhook, err := g.getWebhookByTargetUrl(projectPath, webhookUrl)
	if err != nil {
		return err
	}

	if existingWebhook == nil {
		// Webhook doesn't exist, nothing to do
		return nil
	}

	return g.deleteWebhook(projectPath, existingWebhook.ID)
}

// GetDefaultBranch returns name of default branch in the given repository
func (g *GitlabClient) GetDefaultBranch(repoUrl string) (string, error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return "", err
	}

	return g.getDefaultBranch(projectPath)
}

// DeleteBranch deletes given branch from repository
func (g *GitlabClient) DeleteBranch(repoUrl, branchName string) (bool, error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return false, err
	}
	return g.deleteBranch(projectPath, branchName)
}

// GetBranchSha returns SHA of top commit in the given branch
func (g *GitlabClient) GetBranchSha(repoUrl, branchName string) (string, error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return "", err
	}

	branch, err := g.getBranch(projectPath, branchName)
	if err != nil {
		return "", err
	}
	if branch.Commit == nil {
		return "", fmt.Errorf("unexpected response while getting branch top commit SHA")
	}
	sha := branch.Commit.ID
	return sha, nil
}

// IsFileExist check whether given file exists in the given branch of the reposiotry.
// If branch is empty string, default branch is used.
func (g *GitlabClient) IsFileExist(repoUrl, branchName, filePath string) (bool, error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return false, err
	}

	if branchName == "" {
		var err error
		branchName, err = g.getDefaultBranch(projectPath)
		if err != nil {
			return false, err
		}
	}

	directory := filepath.Dir(filePath)
	files, err := g.filesExistInDirectory(projectPath, branchName, directory, []gp.RepositoryFile{{FullPath: filePath}})
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// IsRepositoryPublic returns true if the repository could be accessed without authentication
func (g *GitlabClient) IsRepositoryPublic(repoUrl string) (bool, error) {
	projectPath, err := getProjectPathFromRepoUrl(repoUrl)
	if err != nil {
		return false, err
	}

	projectInfo, err := g.getProjectInfo(projectPath)
	if err != nil {
		return false, err
	}
	if projectInfo == nil {
		return false, nil
	}
	return projectInfo.Visibility == "public", nil
}

// GetBrowseRepositoryAtShaLink returns web URL of repository state at given SHA
func (g *GitlabClient) GetBrowseRepositoryAtShaLink(repoUrl, sha string) string {
	repoUrl = strings.TrimSuffix(repoUrl, ".git")
	gitSourceUrlParts := strings.Split(repoUrl, "/")
	gitProviderHost := "https://" + gitSourceUrlParts[2]
	gitlabNamespace := gitSourceUrlParts[3]
	gitlabProjectName := gitSourceUrlParts[4]
	projectPath := gitlabNamespace + "/" + gitlabProjectName

	return fmt.Sprintf("%s/%s/-/tree/%s", gitProviderHost, projectPath, sha)
}

func (g *GitlabClient) GetConfiguredGitAppName() (string, string, error) {
	return "", "", fmt.Errorf("GitLab does not support applications")
}

func newGitlabClient(accessToken, baseUrl string) (*GitlabClient, error) {
	glc := &GitlabClient{}
	c, err := gitlab.NewClient(accessToken, gitlab.WithBaseURL(baseUrl))
	if err != nil {
		return nil, err
	}
	glc.client = c

	return glc, nil
}

func newGitlabClientWithBasicAuth(username, password, baseUrl string) (*GitlabClient, error) {
	glc := &GitlabClient{}
	c, err := gitlab.NewBasicAuthClient(username, password, gitlab.WithBaseURL(baseUrl))
	if err != nil {
		return nil, err
	}
	glc.client = c

	return glc, nil
}
