/*
Copyright 2023 Red Hat, Inc.

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

package gitproviderfactory

import (
	"encoding/base64"
	"fmt"
	"github.com/redhat-appstudio/application-service/gitops"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	"github.com/redhat-appstudio/build-service/pkg/git/github"
	"github.com/redhat-appstudio/build-service/pkg/git/gitlab"
	"github.com/redhat-appstudio/build-service/pkg/git/gitprovider"
	"strconv"
)

var CreateGitClient func(gitClientConfig GitClientConfig) (gitprovider.GitProviderClient, error) = createGitClient

type GitClientConfig struct {
	// PacSecretData are the content of Pipelines as Code secret
	PacSecretData map[string][]byte
	// GitProvider is type of the git provider to construct client for.
	// Cannot be obtained from repo repository URL in case of self-hosted solution.
	GitProvider string
	// RepoUrl is the target git repository URL.
	// Used to check that the requirements to access the repository are met,
	// for example, check that the application is installed into given git repository.
	// Ignored for some client configurations, e.g. clients created directly via a token.
	RepoUrl string
	// IsAppInstallationExpected shows whether to expect application installation into the target repository URL.
	// Ignored for clients created directly via a token.
	// Only for simple builds must be set to false.
	IsAppInstallationExpected bool
}

// createGitClient creates new git provider client for the requested config
func createGitClient(gitClientConfig GitClientConfig) (gitprovider.GitProviderClient, error) {
	gitProvider := gitClientConfig.GitProvider
	secretData := gitClientConfig.PacSecretData
	username, usernameExists := secretData["username"]
	password, passwordExists := secretData["password"]
	_, sshKeyExists := secretData["ssh-privatekey"]

	isAppUsed := gitops.IsPaCApplicationConfigured(gitProvider, secretData)

	switch gitProvider {
	case "github":
		if !isAppUsed {
			if passwordExists {
				password, err := base64.StdEncoding.DecodeString(string(password))
				if err != nil {
					return nil, fmt.Errorf("failed to decode password: %w", err)
				}
				if usernameExists {
					username, err := base64.StdEncoding.DecodeString(string(username))
					if err != nil {
						return nil, fmt.Errorf("failed to decode username: %w", err)
					}
					return github.NewGithubClientWithBasicAuth(string(username), string(password)), nil
				}
				return github.NewGithubClient(string(password)), nil
			}
			if sshKeyExists {
				return nil, boerrors.NewBuildOpError(boerrors.EGitHubSecretTypeNotSupported,
					fmt.Errorf("failed to create git client: GitHub ssh key authentication not yet supported"))
			}
			return nil, boerrors.NewBuildOpError(boerrors.EGitHubSecretTypeNotSupported,
				fmt.Errorf("failed to create git client:  unsupported secret data. Expected username/password or token"))
		}

		githubAppIdStr := string(secretData[gitops.PipelinesAsCode_githubAppIdKey])
		githubAppId, err := strconv.ParseInt(githubAppIdStr, 10, 64)
		if err != nil {
			return nil, boerrors.NewBuildOpError(boerrors.EGitHubAppMalformedId,
				fmt.Errorf("failed to create git client: failed to convert %s to int: %w", githubAppIdStr, err))
		}

		privateKey := secretData[gitops.PipelinesAsCode_githubPrivateKey]

		if gitClientConfig.IsAppInstallationExpected {
			// It's required that the configured Pipelines as Code application is installed into user's account
			// and enabled for the given repository.

			githubClient, err := github.NewGithubClientByApp(githubAppId, privateKey, gitClientConfig.RepoUrl)
			if err != nil {
				return nil, err
			}

			// Check if the application is installed into target repository
			appInstalled, err := githubClient.IsAppInstalledIntoRepository(gitClientConfig.RepoUrl)
			if err != nil {
				return nil, err
			}
			if !appInstalled {
				return nil, boerrors.NewBuildOpError(boerrors.EGitHubAppNotInstalled,
					fmt.Errorf("failed to create git client: GitHub Application is not installed into the repository"))
			}
			return githubClient, nil
		} else {
			// For simple builds we need to query repositories where configured Pipelines as Code application is not installed.
			githubClient, err := github.NewGithubClientForSimpleBuildByApp(githubAppId, privateKey)
			if err != nil {
				return nil, err
			}
			return githubClient, nil
		}

	case "gitlab":
		if isAppUsed {
			return nil, fmt.Errorf("GitLab does not have applications")
		}
		baseUrl, err := gitlab.GetBaseUrl(gitClientConfig.RepoUrl)
		if err != nil {
			return nil, err
		}
		if usernameExists && passwordExists {
			username, err := base64.StdEncoding.DecodeString(string(username))
			if err != nil {
				return nil, fmt.Errorf("failed to decode username: %w", err)
			}
			password, err := base64.StdEncoding.DecodeString(string(password))
			if err != nil {
				return nil, fmt.Errorf("failed to decode password: %w", err)
			}
			return gitlab.NewGitlabClientWithBasicAuth(string(username), string(password), baseUrl)
		}
		if !usernameExists && passwordExists {
			password, err := base64.StdEncoding.DecodeString(string(password))
			if err != nil {
				return nil, fmt.Errorf("failed to decode password: %w", err)
			}
			return gitlab.NewGitlabClient(string(password), baseUrl)
		}
		if sshKeyExists {
			return nil, boerrors.NewBuildOpError(boerrors.EGitLabSecretTypeNotSupported,
				fmt.Errorf("failed to create git client: GitLab ssh key authentication not yet supported"))
		}
		return nil, boerrors.NewBuildOpError(boerrors.EGitLabSecretTypeNotSupported,
			fmt.Errorf("failed to create git client: unsupported secret data. Expected username/password or token"))

	case "bitbucket":
		return nil, boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, fmt.Errorf("git provider %s is not supported", gitProvider))
	default:
		return nil, boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, fmt.Errorf("git provider %s is not supported", gitProvider))
	}
}
