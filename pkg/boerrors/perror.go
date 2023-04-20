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

package boerrors

import (
	"fmt"
)

var _ error = (*BuildOpError)(nil)

// BuildOpError extends standard error to:
//  1. Keep persistent / transient property of the error.
//     All errors, except ETransientErrorId considered persistent.
//  2. Have error ID to show the root cause of the error and optionally short message.
type BuildOpError struct {
	// id is used to determine if error is persistent and to know the root cause of the error
	id BOErrorId
	// typically used to log the error message along with nested errors
	err error
	// Optional. To provide extra information about this error
	// If set, it will be appended to the error message returned from Error
	ExtraInfo string
}

func NewBuildOpError(id BOErrorId, err error) *BuildOpError {
	return &BuildOpError{
		id:        id,
		err:       err,
		ExtraInfo: "",
	}
}

func (r BuildOpError) Error() string {
	if r.err == nil {
		return ""
	}
	if r.ExtraInfo == "" {
		return r.err.Error()
	} else {
		return fmt.Sprintf("%s %s", r.err.Error(), r.ExtraInfo)
	}
}

// ShortError returns short message with error ID in case of persistent error or
// standard error message for transient errors.
func (r BuildOpError) ShortError() string {
	if r.id == ETransientError {
		return r.Error()
	}
	return fmt.Sprintf("%d: %s", r.id, boErrorMessages[r.id])
}

func (r BuildOpError) IsPersistent() bool {
	return r.id != ETransientError
}

type BOErrorId int

const (
	ETransientError BOErrorId = 0
	EUnknownError   BOErrorId = 1

	// 'pipelines-as-code-secret' secret doesn't exists in 'build-service' namespace nor Component's one.
	EPaCSecretNotFound BOErrorId = 50
	// Validation of 'pipelines-as-code-secret' secret failed
	EPaCSecretInvalid BOErrorId = 51

	// Happens when Component source repository is hosted on unsupported / unknown git provider.
	// For example: https://my-gitlab.com
	// If self-hosted instance of the supported git providers is used, then "git-provider" annotation must be set:
	// git-provider: gitlab
	EUnknownGitProvider BOErrorId = 60

	// Happens when configured in cluster Pipelines as Code application is not installed in Component source repository.
	// User must install the application to fix this error.
	EGitHubAppNotInstalled BOErrorId = 70
	// Bad formatted private key
	EGitHubAppMalformedPrivateKey BOErrorId = 71
	// Private key doesn't match the GitHub Application
	EGitHubAppPrivateKeyNotMatched BOErrorId = 72
	// GitHub Application with specified ID does not exists.
	// Correct configuration in the AppStudio installation ('pipelines-as-code-secret' secret in 'build-service' namespace).
	EGitHubAppDoesNotExist BOErrorId = 73

	// EGitHubTokenUnauthorized access token can't be recognized by GitHub and 401 is responded.
	// This error may be caused by a malformed token string or an expired token.
	EGitHubTokenUnauthorized BOErrorId = 74
	// EGitHubNoResourceToOperateOn No resource is suitable for GitHub to handle the request and 404 is responded.
	// Generally, this error could be caused by two cases. One is, operate non-existing resource with an access
	// token that has sufficient scope, e.g. delete a non-existing webhook. Another one is, the access token does
	// not have sufficient scope, e.g. list webhooks from a repository, but scope "read:repo_hook" is set.
	EGitHubNoResourceToOperateOn BOErrorId = 75
	// EGitHubReachRateLimit reach the GitHub REST API rate limit.
	EGitHubReachRateLimit BOErrorId = 76

	// EGitLabTokenUnauthorized access token is not recognized by GitLab and 401 is responded.
	// The access token may be malformed or expired.
	EGitLabTokenUnauthorized BOErrorId = 90
	// EGitLabTokenInsufficientScope the access token does not have sufficient scope and 403 is responded.
	EGitLabTokenInsufficientScope BOErrorId = 91

	// Value of 'image.redhat.com/image' component annotation is not a valid json or the json has invalid structure.
	EFailedToParseImageAnnotation BOErrorId = 200
	// The secret with git credentials specified in component.Spec.Secret does not exist in the user's namespace.
	EComponentGitSecretMissing BOErrorId = 201
)

var boErrorMessages = map[BOErrorId]string{
	ETransientError: "",
	EUnknownError:   "unknown error",

	EPaCSecretNotFound: "Pipelines as Code secret does not exist",
	EPaCSecretInvalid:  "Invalid Pipelines as Code secret",

	EUnknownGitProvider: "unknown git provider of the source repository",

	EGitHubAppNotInstalled:         "GitHub Application is not installed in user repository",
	EGitHubAppMalformedPrivateKey:  "invalid GitHub Application private key",
	EGitHubAppPrivateKeyNotMatched: "GitHub Application private key does not match Application ID",
	EGitHubAppDoesNotExist:         "GitHub Application with given ID does not exist",

	EGitHubTokenUnauthorized:     "Access token is unrecognizable by GitHub",
	EGitHubNoResourceToOperateOn: "No resource for finishing the request",
	EGitHubReachRateLimit:        "Reach GitHub REST API rate limit",

	EGitLabTokenInsufficientScope: "GitLab access token does not have enough scope",
	EGitLabTokenUnauthorized:      "Access token is unrecognizable by remote GitLab service",

	EFailedToParseImageAnnotation: "Failed to parse image.redhat.com/image annotation value",
	EComponentGitSecretMissing:    "Specified secret with git credential not found",
}
