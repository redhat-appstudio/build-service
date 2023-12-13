/*
Copyright 2021-2023 Red Hat, Inc.

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

package controllers

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/application-service/gitops"
	gitopsprepare "github.com/redhat-appstudio/application-service/gitops/prepare"
	"github.com/redhat-appstudio/application-service/pkg/devfile"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	gp "github.com/redhat-appstudio/build-service/pkg/git/gitprovider"
	"github.com/redhat-appstudio/build-service/pkg/git/gitproviderfactory"
	l "github.com/redhat-appstudio/build-service/pkg/logs"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonapi_v1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	oci "github.com/tektoncd/pipeline/pkg/remote/oci"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

const (
	PipelineRunOnPRExpirationEnvVar  = "IMAGE_TAG_ON_PR_EXPIRATION"
	PipelineRunOnPRExpirationDefault = "5d"
	pipelineRunOnPushSuffix          = "-on-push"
	pipelineRunOnPRSuffix            = "-on-pull-request"
	pipelineRunOnPushFilename        = "push.yaml"
	pipelineRunOnPRFilename          = "pull-request.yaml"
	pipelinesAsCodeNamespace         = "openshift-pipelines"
	pipelinesAsCodeNamespaceFallback = "pipelines-as-code"
	pipelinesAsCodeRouteName         = "pipelines-as-code-controller"
	pipelinesAsCodeRouteEnvVar       = "PAC_WEBHOOK_URL"

	pacCelExpressionAnnotationName = "pipelinesascode.tekton.dev/on-cel-expression"
	pacIncomingSecretNameSuffix    = "-incoming"
	pacIncomingSecretKey           = "incoming-secret"

	pacMergeRequestSourceBranchPrefix = "appstudio-"

	appstudioWorkspaceNameLabel      = "appstudio.redhat.com/workspace_name"
	pacCustomParamAppstudioWorkspace = "appstudio_workspace"

	mergeRequestDescription = `
# Pipelines as Code configuration proposal

To start the PipelineRun, add a new comment with content ` + "`/ok-to-test`" + `

For more detailed information about running a PipelineRun, please refer to Pipelines as Code documentation [Running the PipelineRun](https://pipelinesascode.com/docs/guide/running/)

To customize the proposed PipelineRuns after merge, please refer to [Build Pipeline customization](https://redhat-appstudio.github.io/docs.appstudio.io/Documentation/main/how-to-guides/configuring-builds/proc_customize_build_pipeline/)
`
)

// That way it can be mocked in tests
var GetHttpClientFunction = getHttpClient

// ProvisionPaCForComponent does Pipelines as Code provision for the given component.
// Mainly, it creates PaC configuration merge request into the component source repositotiry.
// If GitHub PaC application is not configured, creates a webhook for PaC.
func (r *ComponentBuildReconciler) ProvisionPaCForComponent(ctx context.Context, component *appstudiov1alpha1.Component) (string, error) {
	log := ctrllog.FromContext(ctx).WithName("PaC-setup")
	ctx = ctrllog.IntoContext(ctx, log)

	log.Info("Starting Pipelines as Code provision for the Component")

	gitProvider, err := gitops.GetGitProvider(*component)
	if err != nil {
		// Do not reconcile, because configuration must be fixed before it is possible to proceed.
		return "", boerrors.NewBuildOpError(boerrors.EUnknownGitProvider,
			fmt.Errorf("error detecting git provider: %w", err))
	}

	pacSecret, err := r.ensurePaCSecret(ctx, component, gitProvider)
	if err != nil {
		return "", err
	}

	if err := validatePaCConfiguration(gitProvider, pacSecret.Data); err != nil {
		r.EventRecorder.Event(pacSecret, "Warning", "ErrorValidatingPaCSecret", err.Error())
		// Do not reconcile, because configuration must be fixed before it is possible to proceed.
		return "", boerrors.NewBuildOpError(boerrors.EPaCSecretInvalid,
			fmt.Errorf("invalid configuration in Pipelines as Code secret: %w", err))
	}

	var webhookSecretString, webhookTargetUrl string
	if !gitops.IsPaCApplicationConfigured(gitProvider, pacSecret.Data) {
		// Generate webhook secret for the component git repository if not yet generated
		// and stores it in the corresponding k8s secret.
		webhookSecretString, err = r.ensureWebhookSecret(ctx, component)
		if err != nil {
			return "", err
		}

		// Obtain Pipelines as Code callback URL
		webhookTargetUrl, err = r.getPaCWebhookTargetUrl(ctx)
		if err != nil {
			return "", err
		}
	}

	if err := r.ensurePaCRepository(ctx, component, pacSecret.Data); err != nil {
		return "", err
	}

	// Manage merge request for Pipelines as Code configuration
	mrUrl, err := r.ConfigureRepositoryForPaC(ctx, component, pacSecret.Data, webhookTargetUrl, webhookSecretString)
	if err != nil {
		r.EventRecorder.Event(component, "Warning", "ErrorConfiguringPaCForComponentRepository", err.Error())
		return "", err
	}
	var mrMessage string
	if mrUrl != "" {
		mrMessage = fmt.Sprintf("Pipelines as Code configuration merge request: %s", mrUrl)
	} else {
		mrMessage = "Pipelines as Code configuration is up to date"
	}
	log.Info(mrMessage)
	r.EventRecorder.Event(component, "Normal", "PipelinesAsCodeConfiguration", mrMessage)

	return mrUrl, nil
}

func getHttpClient() *http.Client { // #nosec G402 // dev instances need insecure, because they have self signed certificates
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: gp.IsInsecureSSL()},
	}
	client := &http.Client{Transport: tr}
	return client
}

// ensureIncomingSecret is ensuring that incoming secret for PaC trigger exists
// if secret doesn't exists it will create it and also add repository as owner
// Returns:
// pointer to secret object
// bool which indicates if reconcile is required (which is required when we just created secret)
func (r *ComponentBuildReconciler) ensureIncomingSecret(ctx context.Context, component *appstudiov1alpha1.Component) (*corev1.Secret, bool, error) {
	log := ctrllog.FromContext(ctx)

	repository, err := r.findPaCRepositoryForComponent(ctx, component)
	if err != nil {
		return nil, false, err
	}

	incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)
	incomingSecretPassword := generatePaCWebhookSecretString()
	incomingSecretData := map[string]string{
		pacIncomingSecretKey: incomingSecretPassword,
	}

	secret := corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: component.Namespace, Name: incomingSecretName}, &secret); err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get incoming secret", l.Action, l.ActionView)
			return nil, false, err
		}
		// Create incoming secret
		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      incomingSecretName,
				Namespace: component.Namespace,
			},
			Type:       corev1.SecretTypeOpaque,
			StringData: incomingSecretData,
		}

		if err := controllerutil.SetOwnerReference(repository, &secret, r.Scheme); err != nil {
			log.Error(err, "failed to set owner for incoming secret")
			return nil, false, err
		}

		if err := r.Client.Create(ctx, &secret); err != nil {
			log.Error(err, "failed to create incoming secret", l.Action, l.ActionAdd)
			return nil, false, err
		}

		log.Info("incoming secret created")
		return &secret, true, nil
	}
	return &secret, false, nil
}

func (r *ComponentBuildReconciler) TriggerPaCBuild(ctx context.Context, component *appstudiov1alpha1.Component) (bool, error) {
	log := ctrllog.FromContext(ctx).WithName("TriggerPaCBuild")
	ctx = ctrllog.IntoContext(ctx, log)

	incomingSecret, reconcileRequired, err := r.ensureIncomingSecret(ctx, component)
	if err != nil {
		return false, err
	}

	repository, err := r.findPaCRepositoryForComponent(ctx, component)
	if err != nil {
		return false, err
	}

	if repository == nil {
		return false, fmt.Errorf("PaC repository not found for component %s", component.Name)
	}

	repoUrl := component.Spec.Source.GitSource.URL
	gitProvider, err := gitops.GetGitProvider(*component)
	if err != nil {
		log.Error(err, "error detecting git provider")
		// There is no point to continue if git provider is not known.
		return false, boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, err)
	}

	pacSecret, err := r.ensurePaCSecret(ctx, component, gitProvider)
	if err != nil {
		return false, err
	}

	gitClient, err := gitproviderfactory.CreateGitClient(gitproviderfactory.GitClientConfig{
		PacSecretData:             pacSecret.Data,
		GitProvider:               gitProvider,
		RepoUrl:                   repoUrl,
		IsAppInstallationExpected: true,
	})
	if err != nil {
		return false, err
	}

	// get target branch for incoming hook
	targetBranch := component.Spec.Source.GitSource.Revision
	if targetBranch == "" {
		targetBranch, err = gitClient.GetDefaultBranch(repoUrl)
		if err != nil {
			return false, err
		}
	}

	incomingUpdated := updateIncoming(repository, incomingSecret.Name, pacIncomingSecretKey, targetBranch)
	if incomingUpdated {
		if err := r.Client.Update(ctx, repository); err != nil {
			log.Error(err, "failed to update PaC repository with incomings", "PaCRepositoryName", repository.Name)
			return false, err
		}
		log.Info("Added incomings to the PaC repository", "PaCRepositoryName", repository.Name, l.Action, l.ActionUpdate)

		// reconcile to be sure that Repository is updated, as Repository needs to have correct incomings for trigger to work
		return true, nil
	}

	// reconcile to be sure that Secret is created
	if reconcileRequired {
		return true, nil
	}

	webhookTargetUrl, err := r.getPaCWebhookTargetUrl(ctx)
	if err != nil {
		return false, err
	}

	secretValue := string(incomingSecret.Data[pacIncomingSecretKey][:])

	pipelineRunName := component.Name + pipelineRunOnPushSuffix

	triggerURL := fmt.Sprintf("%s/incoming?secret=%s&repository=%s&branch=%s&pipelinerun=%s", webhookTargetUrl, secretValue, repository.Name, targetBranch, pipelineRunName)
	HttpClient := GetHttpClientFunction()

	resp, err := HttpClient.Post(triggerURL, "application/json", nil)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return false, fmt.Errorf("PaC incoming endpoint returned HTTP %d", resp.StatusCode)
	}

	log.Info(fmt.Sprintf("PaC build manually triggered push pipeline for component: %s", component.Name))
	return false, nil
}

// cleanupPaCRepositoryIncomingsAndSecret is cleaning up incomings in Repository
// for unprovisioned component, and also removes incoming secret when no longer required
func (r *ComponentBuildReconciler) cleanupPaCRepositoryIncomingsAndSecret(ctx context.Context, component *appstudiov1alpha1.Component, baseBranch string) error {
	log := ctrllog.FromContext(ctx)

	// check if more components are using same repo with PaC enabled for incomings removal from repository
	incomingsRepoTargetBranchCount := 0
	incomingsRepoAllBranchesCount := 0
	componentList := &appstudiov1alpha1.ComponentList{}
	if err := r.Client.List(ctx, componentList, &client.ListOptions{Namespace: component.Namespace}); err != nil {
		log.Error(err, "failed to list Components", l.Action, l.ActionView)
		return err
	}
	buildStatus := &BuildStatus{}
	for _, comp := range componentList.Items {
		if comp.Spec.Source.GitSource.URL == component.Spec.Source.GitSource.URL {
			buildStatus = readBuildStatus(component)
			if buildStatus.PaC != nil && buildStatus.PaC.State == "enabled" {
				incomingsRepoAllBranchesCount += 1

				// revision can be empty and then use default branch
				if comp.Spec.Source.GitSource.Revision == component.Spec.Source.GitSource.Revision || comp.Spec.Source.GitSource.Revision == baseBranch {
					incomingsRepoTargetBranchCount += 1
				}
			}
		}
	}

	repository, err := r.findPaCRepositoryForComponent(ctx, component)
	if err != nil {
		return err
	}

	// repository is used to construct incoming secret name
	if repository == nil {
		return nil
	}

	incomingSecretName := fmt.Sprintf("%s%s", repository.Name, pacIncomingSecretNameSuffix)
	incomingUpdated := false
	// update first in case there is multiple incoming entries, and it will be converted to incomings with just 1 entry
	_ = updateIncoming(repository, incomingSecretName, pacIncomingSecretKey, baseBranch)

	if len((*repository.Spec.Incomings)[0].Targets) > 1 {
		// incoming contains target from the current component only
		if slices.Contains((*repository.Spec.Incomings)[0].Targets, baseBranch) && incomingsRepoTargetBranchCount <= 1 {
			newTargets := []string{}
			for _, target := range (*repository.Spec.Incomings)[0].Targets {
				if target != baseBranch {
					newTargets = append(newTargets, target)
				}
			}
			(*repository.Spec.Incomings)[0].Targets = newTargets
			incomingUpdated = true
		}
		// remove secret from incomings if just current component is using incomings in repository
		if incomingsRepoAllBranchesCount <= 1 && incomingsRepoTargetBranchCount <= 1 {
			(*repository.Spec.Incomings)[0].Secret = pacv1alpha1.Secret{}
			incomingUpdated = true
		}

	} else {
		// incomings has just 1 target and that target is from the current component only
		if (*repository.Spec.Incomings)[0].Targets[0] == baseBranch && incomingsRepoTargetBranchCount <= 1 {
			repository.Spec.Incomings = nil
			incomingUpdated = true
		}
	}

	if incomingUpdated {
		if err := r.Client.Update(ctx, repository); err != nil {
			log.Error(err, "failed to update existing PaC repository with incomings", "PaCRepositoryName", repository.Name)
			return err
		}
		log.Info("Removed incomings from the PaC repository", "PaCRepositoryName", repository.Name, l.Action, l.ActionUpdate)
	}

	// remove incoming secret if just current component is using incomings in repository
	if incomingsRepoAllBranchesCount <= 1 && incomingsRepoTargetBranchCount <= 1 {
		secret := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: component.Namespace, Name: incomingSecretName}, secret); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "failed to get incoming secret", l.Action, l.ActionView)
				return err
			}
			log.Info("incoming secret doesn't exist anymore, removal isn't required")
		} else {
			if err := r.Client.Delete(ctx, secret); err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "failed to remove incoming secret", l.Action, l.ActionView)
					return err
				}
			}
			log.Info("incoming secret removed")
		}
	}
	return nil
}

// UndoPaCProvisionForComponent creates merge request that removes Pipelines as Code configuration from component source repository.
// Deletes PaC webhook if used.
// In case of any errors just logs them and does not block Component deletion.
func (r *ComponentBuildReconciler) UndoPaCProvisionForComponent(ctx context.Context, component *appstudiov1alpha1.Component) (string, error) {
	log := ctrllog.FromContext(ctx).WithName("PaC-cleanup")
	ctx = ctrllog.IntoContext(ctx, log)

	log.Info("Starting Pipelines as Code unprovision for the Component")

	gitProvider, err := gitops.GetGitProvider(*component)
	if err != nil {
		log.Error(err, "error detecting git provider")
		// There is no point to continue if git provider is not known.
		return "", boerrors.NewBuildOpError(boerrors.EUnknownGitProvider, err)
	}

	pacSecret := corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: buildServiceNamespaceName, Name: gitopsprepare.PipelinesAsCodeSecretName}, &pacSecret); err != nil {
		log.Error(err, "error getting git provider credentials secret", l.Action, l.ActionView)
		// Cannot continue without accessing git provider credentials.
		return "", boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound, err)
	}

	webhookTargetUrl := ""
	if !gitops.IsPaCApplicationConfigured(gitProvider, pacSecret.Data) {
		webhookTargetUrl, err = r.getPaCWebhookTargetUrl(ctx)
		if err != nil {
			// Just log the error and continue with pruning merge request creation
			log.Error(err, "failed to get Pipelines as Code webhook target URL. Webhook will not be deleted.", l.Action, l.ActionView, l.Audit, "true")
		}
	}

	// Manage merge request for Pipelines as Code configuration removal
	baseBranch, mrUrl, action, err := r.UnconfigureRepositoryForPaC(ctx, component, pacSecret.Data, webhookTargetUrl)
	if err != nil {
		log.Error(err, "failed to create merge request to remove Pipelines as Code configuration from Component source repository", l.Audit, "true")
		return "", err
	}

	err = r.cleanupPaCRepositoryIncomingsAndSecret(ctx, component, baseBranch)
	if err != nil {
		log.Error(err, "failed cleanup incomings from repo and incoming secret")
		return "", err
	}

	if action == "delete" {
		if mrUrl != "" {
			log.Info(fmt.Sprintf("Pipelines as Code configuration removal merge request: %s", mrUrl))
		} else {
			log.Info("Pipelines as Code configuration removal merge request is not needed")
		}
	} else if action == "close" {
		log.Info(fmt.Sprintf("Pipelines as Code configuration merge request has been closed: %s", mrUrl))
	}
	return mrUrl, nil
}

func (r *ComponentBuildReconciler) ensurePaCSecret(ctx context.Context, component *appstudiov1alpha1.Component, gitProvider string) (*corev1.Secret, error) {
	// Expected that the secret contains token for Pipelines as Code webhook configuration,
	// but under <git-provider>.token field. For example: github.token
	// Also it can contain github-private-key and github-application-id
	// in case GitHub Application is used instead of webhook.
	pacSecret := corev1.Secret{}
	pacSecretKey := types.NamespacedName{Namespace: component.Namespace, Name: gitopsprepare.PipelinesAsCodeSecretName}
	if err := r.Client.Get(ctx, pacSecretKey, &pacSecret); err != nil {
		if !errors.IsNotFound(err) {
			r.EventRecorder.Event(&pacSecret, "Warning", "ErrorReadingPaCSecret", err.Error())
			return nil, fmt.Errorf("failed to get Pipelines as Code secret in %s namespace: %w", component.Namespace, err)
		}

		// Fallback to the global configuration
		globalPaCSecretKey := types.NamespacedName{Namespace: buildServiceNamespaceName, Name: gitopsprepare.PipelinesAsCodeSecretName}
		if err := r.Client.Get(ctx, globalPaCSecretKey, &pacSecret); err != nil {
			if !errors.IsNotFound(err) {
				r.EventRecorder.Event(&pacSecret, "Warning", "ErrorReadingPaCSecret", err.Error())
				return nil, fmt.Errorf("failed to get Pipelines as Code secret in %s namespace: %w", globalPaCSecretKey.Namespace, err)
			}

			r.EventRecorder.Event(&pacSecret, "Warning", "PaCSecretNotFound", err.Error())
			// Do not trigger a new reconcile. The PaC secret must be created first.
			return nil, boerrors.NewBuildOpError(boerrors.EPaCSecretNotFound,
				fmt.Errorf(" Pipelines as Code secret not found in %s namespace nor in %s", pacSecretKey.Namespace, globalPaCSecretKey.Namespace))
		}

		if !gitops.IsPaCApplicationConfigured(gitProvider, pacSecret.Data) {
			// Webhook is used. We need to reference access token in the component namespace.
			// Copy global PaC configuration in component namespace
			localPaCSecret := &corev1.Secret{
				TypeMeta: pacSecret.TypeMeta,
				ObjectMeta: metav1.ObjectMeta{
					Name:      pacSecretKey.Name,
					Namespace: pacSecretKey.Namespace,
					Labels: map[string]string{
						PartOfLabelName: PartOfAppStudioLabelValue,
					},
				},
				Data: pacSecret.Data,
			}
			if err := r.Client.Create(ctx, localPaCSecret); err != nil {
				return nil, fmt.Errorf("failed to create local PaC configuration secret: %w", err)
			}
		}
	}

	return &pacSecret, nil
}

// Returns webhook secret for given component.
// Generates the webhook secret and saves it the k8s secret if doesn't exist.
func (r *ComponentBuildReconciler) ensureWebhookSecret(ctx context.Context, component *appstudiov1alpha1.Component) (string, error) {
	log := ctrllog.FromContext(ctx)

	webhookSecretsSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: gitops.PipelinesAsCodeWebhooksSecretName, Namespace: component.GetNamespace()}, webhookSecretsSecret); err != nil {
		if errors.IsNotFound(err) {
			webhookSecretsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitops.PipelinesAsCodeWebhooksSecretName,
					Namespace: component.GetNamespace(),
					Labels: map[string]string{
						PartOfLabelName: PartOfAppStudioLabelValue,
					},
				},
			}
			if err := r.Client.Create(ctx, webhookSecretsSecret); err != nil {
				log.Error(err, "failed to create webhooks secrets secret", l.Action, l.ActionAdd)
				return "", err
			}
			return r.ensureWebhookSecret(ctx, component)
		}

		log.Error(err, "failed to get webhook secrets secret", l.Action, l.ActionView)
		return "", err
	}

	componentWebhookSecretKey := gitops.GetWebhookSecretKeyForComponent(*component)
	if _, exists := webhookSecretsSecret.Data[componentWebhookSecretKey]; exists {
		// The webhook secret already exists. Use single secret for the same repository.
		return string(webhookSecretsSecret.Data[componentWebhookSecretKey]), nil
	}

	webhookSecretString := generatePaCWebhookSecretString()

	if webhookSecretsSecret.Data == nil {
		webhookSecretsSecret.Data = make(map[string][]byte)
	}
	webhookSecretsSecret.Data[componentWebhookSecretKey] = []byte(webhookSecretString)
	if err := r.Client.Update(ctx, webhookSecretsSecret); err != nil {
		log.Error(err, "failed to update webhook secrets secret", l.Action, l.ActionUpdate)
		return "", err
	}

	return webhookSecretString, nil
}

// generatePaCWebhookSecretString generates string alike openssl rand -hex 20
func generatePaCWebhookSecretString() string {
	length := 20 // in bytes
	tokenBytes := make([]byte, length)
	if _, err := rand.Read(tokenBytes); err != nil {
		panic("Failed to read from random generator")
	}
	return hex.EncodeToString(tokenBytes)
}

// getPaCWebhookTargetUrl returns URL to which events from git repository should be sent.
func (r *ComponentBuildReconciler) getPaCWebhookTargetUrl(ctx context.Context) (string, error) {
	webhookTargetUrl := os.Getenv(pipelinesAsCodeRouteEnvVar)
	if webhookTargetUrl == "" {
		// The env variable is not set
		// Use the installed on the cluster Pipelines as Code
		var err error
		webhookTargetUrl, err = r.getPaCRoutePublicUrl(ctx)
		if err != nil {
			return "", err
		}
	}
	return webhookTargetUrl, nil
}

// getPaCRoutePublicUrl returns Pipelines as Code public route that recieves events to trigger new pipeline runs.
func (r *ComponentBuildReconciler) getPaCRoutePublicUrl(ctx context.Context) (string, error) {
	pacWebhookRoute := &routev1.Route{}
	pacWebhookRouteKey := types.NamespacedName{Namespace: pipelinesAsCodeNamespace, Name: pipelinesAsCodeRouteName}
	if err := r.Client.Get(ctx, pacWebhookRouteKey, pacWebhookRoute); err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to get Pipelines as Code route in %s namespace: %w", pacWebhookRouteKey.Namespace, err)
		}
		// Fallback to old PaC namesapce
		pacWebhookRouteKey.Namespace = pipelinesAsCodeNamespaceFallback
		if err := r.Client.Get(ctx, pacWebhookRouteKey, pacWebhookRoute); err != nil {
			if !errors.IsNotFound(err) {
				return "", fmt.Errorf("failed to get Pipelines as Code route in %s namespace: %w", pacWebhookRouteKey.Namespace, err)
			}
			// Pipelines as Code public route was not found in expected namespaces
			// Consider this error permanent
			return "", boerrors.NewBuildOpError(boerrors.EPaCRouteDoesNotExist,
				fmt.Errorf("PaC route not found in %s nor %s namespace", pipelinesAsCodeNamespace, pipelinesAsCodeNamespaceFallback))
		}
	}
	return "https://" + pacWebhookRoute.Spec.Host, nil
}

// validatePaCConfiguration detects checks that all required fields is set for whatever method is used.
func validatePaCConfiguration(gitProvider string, config map[string][]byte) error {
	isApp := gitops.IsPaCApplicationConfigured(gitProvider, config)

	expectedPaCWebhookConfigFields := []string{gitops.GetProviderTokenKey(gitProvider)}

	var err error
	switch gitProvider {
	case "github":
		if isApp {
			// GitHub application

			err = checkMandatoryFieldsNotEmpty(config, []string{gitops.PipelinesAsCode_githubAppIdKey, gitops.PipelinesAsCode_githubPrivateKey})
			if err != nil {
				break
			}

			// validate content of the fields
			if _, e := strconv.ParseInt(string(config[gitops.PipelinesAsCode_githubAppIdKey]), 10, 64); e != nil {
				err = fmt.Errorf(" Pipelines as Code: failed to parse GitHub application ID. Cause: %w", e)
				break
			}

			privateKey := strings.TrimSpace(string(config[gitops.PipelinesAsCode_githubPrivateKey]))
			if !strings.HasPrefix(privateKey, "-----BEGIN RSA PRIVATE KEY-----") ||
				!strings.HasSuffix(privateKey, "-----END RSA PRIVATE KEY-----") {
				err = fmt.Errorf(" Pipelines as Code secret: GitHub application private key is invalid")
				break
			}
		} else {
			// webhook
			err = checkMandatoryFieldsNotEmpty(config, expectedPaCWebhookConfigFields)
		}

	case "gitlab":
		err = checkMandatoryFieldsNotEmpty(config, expectedPaCWebhookConfigFields)

	case "bitbucket":
		err = checkMandatoryFieldsNotEmpty(config, []string{gitops.GetProviderTokenKey(gitProvider)})
		if err != nil {
			break
		}

		if len(config["username"]) == 0 {
			err = fmt.Errorf(" Pipelines as Code secret: name of the user field must be configured")
		}

	default:
		err = fmt.Errorf("unsupported git provider: %s", gitProvider)
	}

	return err
}

func checkMandatoryFieldsNotEmpty(config map[string][]byte, mandatoryFields []string) error {
	for _, field := range mandatoryFields {
		if len(config[field]) == 0 {
			return fmt.Errorf(" Pipelines as Code secret: %s field is not configured", field)
		}
	}
	return nil
}

// pacRepoAddParamWorkspaceName adds custom parameter workspace name to a PaC repository.
// Existing parameter will be overridden.
func pacRepoAddParamWorkspaceName(log logr.Logger, repository *pacv1alpha1.Repository, workspaceName string) {
	var params []pacv1alpha1.Params
	// Before pipelines-as-code gets upgraded for application-service to the
	// version supporting custom parameters, this check must be taken.
	if repository.Spec.Params == nil {
		params = make([]pacv1alpha1.Params, 0)
	} else {
		params = *repository.Spec.Params
	}
	found := -1
	for i, param := range params {
		if param.Name == pacCustomParamAppstudioWorkspace {
			found = i
			break
		}
	}
	workspaceParam := pacv1alpha1.Params{
		Name:  pacCustomParamAppstudioWorkspace,
		Value: workspaceName,
	}
	if found >= 0 {
		params[found] = workspaceParam
	} else {
		params = append(params, workspaceParam)
	}
	repository.Spec.Params = &params
}

func (r *ComponentBuildReconciler) getNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: name}, ns)
	if err == nil {
		return ns, nil
	} else {
		return nil, err
	}
}

func (r *ComponentBuildReconciler) ensurePaCRepository(ctx context.Context, component *appstudiov1alpha1.Component, pacConfig map[string][]byte) error {
	log := ctrllog.FromContext(ctx)

	// Check multi component git repository scenario.
	// It's not possible to determine multi component git repository scenario by context directory field,
	// therefore it's required to do the check for all components.
	// For example, there are several dockerfiles in the same git repository
	// and each of them builds separate component from the common codebase.
	// Another scenario is component per branch.
	repository, err := r.findPaCRepositoryForComponent(ctx, component)
	if err != nil {
		return err
	}
	if repository != nil {
		pacRepositoryOwnersNumber := len(repository.OwnerReferences)
		if err := controllerutil.SetOwnerReference(component, repository, r.Scheme); err != nil {
			log.Error(err, "failed to add owner reference to existing PaC repository", "PaCRepositoryName", repository.Name)
			return err
		}
		if len(repository.OwnerReferences) > pacRepositoryOwnersNumber {
			if err := r.Client.Update(ctx, repository); err != nil {
				log.Error(err, "failed to update existing PaC repository with component owner reference", "PaCRepositoryName", repository.Name)
				return err
			}
			log.Info("Added current component to owners of the PaC repository", "PaCRepositoryName", repository.Name, l.Action, l.ActionUpdate)
		} else {
			log.Info("Using existing PaC Repository object for the component", "PaCRepositoryName", repository.Name)
		}
		return nil
	}

	// This is the first Component that does PaC provision for the git repository
	repository, err = gitops.GeneratePACRepository(*component, pacConfig)
	if err != nil {
		return err
	}

	ns, err := r.getNamespace(ctx, component.GetNamespace())
	if err != nil {
		log.Error(err, "failed to get the component namespace for setting custom parameter.")
		return err
	}
	if val, ok := ns.Labels[appstudioWorkspaceNameLabel]; ok {
		pacRepoAddParamWorkspaceName(log, repository, val)
	}

	existingRepository := &pacv1alpha1.Repository{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: repository.Name, Namespace: repository.Namespace}, existingRepository); err != nil {
		if errors.IsNotFound(err) {
			if err := controllerutil.SetOwnerReference(component, repository, r.Scheme); err != nil {
				return err
			}
			if err := r.Client.Create(ctx, repository); err != nil {
				if strings.Contains(err.Error(), "repository already exist with url") {
					// PaC admission webhook denied creation of the PaC repository,
					// because PaC repository object that references the same git repository already exists.
					log.Info("An attempt to create second PaC Repository for the same git repository", "GitRepository", repository.Spec.URL, l.Action, l.ActionAdd, l.Audit, "true")
					return boerrors.NewBuildOpError(boerrors.EPaCDuplicateRepository, err)
				}
				log.Error(err, "failed to create Component PaC repository object", l.Action, l.ActionAdd)
				return err
			} else {
				log.Info("Created PaC Repository object for the component")
			}
		} else {
			log.Error(err, "failed to get Component PaC repository object", l.Action, l.ActionView)
			return err
		}
	}
	return nil
}

// findPaCRepositoryForComponent searches for existing matching PaC repository object for given component.
// The search makes sense only in the same namespace.
func (r *ComponentBuildReconciler) findPaCRepositoryForComponent(ctx context.Context, component *appstudiov1alpha1.Component) (*pacv1alpha1.Repository, error) {
	log := ctrllog.FromContext(ctx)

	pacRepositoriesList := &pacv1alpha1.RepositoryList{}
	err := r.Client.List(ctx, pacRepositoriesList, &client.ListOptions{Namespace: component.Namespace})
	if err != nil {
		log.Error(err, "failed to list PaC repositories")
		return nil, err
	}

	gitUrl := strings.TrimSuffix(strings.TrimSuffix(component.Spec.Source.GitSource.URL, ".git"), "/")
	for _, pacRepository := range pacRepositoriesList.Items {
		if pacRepository.Spec.URL == gitUrl {
			return &pacRepository, nil
		}
	}
	return nil, nil
}

// generatePaCPipelineRunConfigs generates PipelineRun YAML configs for given component.
// The generated PipelineRun Yaml content are returned in byte string and in the order of push and pull request.
func (r *ComponentBuildReconciler) generatePaCPipelineRunConfigs(ctx context.Context, component *appstudiov1alpha1.Component, gitClient gp.GitProviderClient, pacTargetBranch string) ([]byte, []byte, error) {
	log := ctrllog.FromContext(ctx)

	pipelineRef, additionalPipelineParams, err := r.GetPipelineForComponent(ctx, component)
	if err != nil {
		return nil, nil, err
	}
	pipelineName, pipelineBundle, err := getPipelineNameAndBundle(pipelineRef)
	if err != nil {
		return nil, nil, err
	}
	log.Info(fmt.Sprintf("Selected %s pipeline from %s bundle for %s component",
		pipelineName, pipelineBundle, component.Name),
		l.Audit, "true")

	// Get pipeline from the bundle to be expanded to the PipelineRun
	pipelineSpec, err := retrievePipelineSpec(ctx, pipelineBundle, pipelineName)
	if err != nil {
		r.EventRecorder.Event(component, "Warning", "ErrorGettingPipelineFromBundle", err.Error())
		return nil, nil, err
	}

	pipelineRunOnPush, err := generatePaCPipelineRunForComponent(
		component, pipelineSpec, additionalPipelineParams, false, pacTargetBranch, gitClient)
	if err != nil {
		return nil, nil, err
	}
	pipelineRunOnPushYaml, err := yaml.Marshal(pipelineRunOnPush)
	if err != nil {
		return nil, nil, err
	}

	pipelineRunOnPR, err := generatePaCPipelineRunForComponent(
		component, pipelineSpec, additionalPipelineParams, true, pacTargetBranch, gitClient)
	if err != nil {
		return nil, nil, err
	}
	pipelineRunOnPRYaml, err := yaml.Marshal(pipelineRunOnPR)
	if err != nil {
		return nil, nil, err
	}

	return pipelineRunOnPushYaml, pipelineRunOnPRYaml, nil
}

func generateMergeRequestSourceBranch(component *appstudiov1alpha1.Component) string {
	return fmt.Sprintf("%s%s", pacMergeRequestSourceBranchPrefix, component.Name)
}

// ConfigureRepositoryForPaC creates a merge request with initial Pipelines as Code configuration
// and configures a webhook to notify in-cluster PaC unless application (on the repository side) is used.
func (r *ComponentBuildReconciler) ConfigureRepositoryForPaC(ctx context.Context, component *appstudiov1alpha1.Component, pacConfig map[string][]byte, webhookTargetUrl, webhookSecret string) (prUrl string, err error) {
	log := ctrllog.FromContext(ctx).WithValues("repository", component.Spec.Source.GitSource.URL)
	ctx = ctrllog.IntoContext(ctx, log)

	gitProvider, _ := gitops.GetGitProvider(*component)
	repoUrl := component.Spec.Source.GitSource.URL

	gitClient, err := gitproviderfactory.CreateGitClient(gitproviderfactory.GitClientConfig{
		PacSecretData:             pacConfig,
		GitProvider:               gitProvider,
		RepoUrl:                   repoUrl,
		IsAppInstallationExpected: true,
	})
	if err != nil {
		return "", err
	}

	baseBranch := component.Spec.Source.GitSource.Revision
	if baseBranch == "" {
		baseBranch, err = gitClient.GetDefaultBranch(repoUrl)
		if err != nil {
			return "", err
		}
	}

	pipelineRunOnPushYaml, pipelineRunOnPRYaml, err := r.generatePaCPipelineRunConfigs(ctx, component, gitClient, baseBranch)
	if err != nil {
		return "", err
	}

	mrData := &gp.MergeRequestData{
		CommitMessage:  "Appstudio update " + component.Name,
		BranchName:     generateMergeRequestSourceBranch(component),
		BaseBranchName: baseBranch,
		Title:          "Appstudio update " + component.Name,
		Text:           mergeRequestDescription,
		AuthorName:     "redhat-appstudio",
		AuthorEmail:    "rhtap@redhat.com",
		Files: []gp.RepositoryFile{
			{FullPath: ".tekton/" + component.Name + "-" + pipelineRunOnPushFilename, Content: pipelineRunOnPushYaml},
			{FullPath: ".tekton/" + component.Name + "-" + pipelineRunOnPRFilename, Content: pipelineRunOnPRYaml},
		},
	}

	isAppUsed := gitops.IsPaCApplicationConfigured(gitProvider, pacConfig)
	if isAppUsed {
		// Customize PR data to reflect git application name
		if appName, appSlug, err := gitClient.GetConfiguredGitAppName(); err == nil {
			mrData.CommitMessage = fmt.Sprintf("%s update %s", appName, component.Name)
			mrData.Title = fmt.Sprintf("%s update %s", appName, component.Name)
			mrData.AuthorName = appSlug
		} else {
			if gitProvider == "github" {
				log.Error(err, "failed to get PaC GitHub Application name", l.Action, l.ActionView, l.Audit, "true")
				// Do not fail PaC provision if failed to read GitHub App info
			}
		}
	} else {
		// Webhook
		if err := gitClient.SetupPaCWebhook(repoUrl, webhookTargetUrl, webhookSecret); err != nil {
			log.Error(err, fmt.Sprintf("failed to setup Pipelines as Code webhook %s", webhookTargetUrl), l.Audit, "true")
			return "", err
		} else {
			log.Info(fmt.Sprintf("Pipelines as Code webhook \"%s\" configured for %s Component in %s namespace",
				webhookTargetUrl, component.GetName(), component.GetNamespace()),
				l.Audit, "true")
		}
	}

	return gitClient.EnsurePaCMergeRequest(repoUrl, mrData)
}

// UnconfigureRepositoryForPaC creates a merge request that deletes Pipelines as Code configuration of the diven component in its repository.
// Deletes PaC webhook if it's used.
// Does not delete PaC GitHub application from the repository as its installation was done manually by the user.
// Returns merge request web URL or empty string if it's not needed.
func (r *ComponentBuildReconciler) UnconfigureRepositoryForPaC(ctx context.Context, component *appstudiov1alpha1.Component, pacConfig map[string][]byte, webhookTargetUrl string) (baseBranch string, prUrl string, action string, err error) {
	log := ctrllog.FromContext(ctx)

	gitProvider, _ := gitops.GetGitProvider(*component)
	repoUrl := component.Spec.Source.GitSource.URL

	gitClient, err := gitproviderfactory.CreateGitClient(gitproviderfactory.GitClientConfig{
		PacSecretData:             pacConfig,
		GitProvider:               gitProvider,
		RepoUrl:                   repoUrl,
		IsAppInstallationExpected: true,
	})
	if err != nil {
		return "", "", "", err
	}

	isAppUsed := gitops.IsPaCApplicationConfigured(gitProvider, pacConfig)
	if !isAppUsed {
		if webhookTargetUrl != "" {
			err = gitClient.DeletePaCWebhook(repoUrl, webhookTargetUrl)
			if err != nil {
				// Just log the error and continue with merge request creation
				log.Error(err, fmt.Sprintf("failed to delete Pipelines as Code webhook %s", webhookTargetUrl), l.Action, l.ActionDelete, l.Audit, "true")
			} else {
				log.Info(fmt.Sprintf("Pipelines as Code webhook \"%s\" deleted for %s Component in %s namespace",
					webhookTargetUrl, component.GetName(), component.GetNamespace()),
					l.Action, l.ActionDelete)
			}
		}
	}

	sourceBranch := generateMergeRequestSourceBranch(component)
	baseBranch = component.Spec.Source.GitSource.Revision
	if baseBranch == "" {
		baseBranch, err = gitClient.GetDefaultBranch(repoUrl)
		if err != nil {
			return "", "", "", nil
		}
	}

	mrData := &gp.MergeRequestData{
		BranchName:     sourceBranch,
		BaseBranchName: baseBranch,
		AuthorName:     "redhat-appstudio",
	}

	mergeRequest, err := gitClient.FindUnmergedPaCMergeRequest(repoUrl, mrData)
	if err != nil {
		return baseBranch, "", "", err
	}

	if mergeRequest == nil {
		// Create new PaC configuration clean up merge request
		mrData = &gp.MergeRequestData{
			CommitMessage:  "Appstudio purge " + component.Name,
			BranchName:     "appstudio-purge-" + component.Name,
			BaseBranchName: baseBranch,
			Title:          "Appstudio purge " + component.Name,
			Text:           "Pipelines as Code configuration removal",
			AuthorName:     "redhat-appstudio",
			AuthorEmail:    "rhtap@redhat.com",
			Files: []gp.RepositoryFile{
				{FullPath: ".tekton/" + component.Name + "-" + pipelineRunOnPushFilename},
				{FullPath: ".tekton/" + component.Name + "-" + pipelineRunOnPRFilename},
			},
		}

		if isAppUsed {
			// Customize PR data to reflect git application name
			if appName, appSlug, err := gitClient.GetConfiguredGitAppName(); err == nil {
				mrData.CommitMessage = fmt.Sprintf("%s purge %s", appName, component.Name)
				mrData.Title = fmt.Sprintf("%s purge %s", appName, component.Name)
				mrData.AuthorName = appSlug
			} else {
				if gitProvider == "github" {
					log.Error(err, "failed to get PaC GitHub Application name", l.Action, l.ActionView, l.Audit, "true")
					// Do not fail PaC clean up PR if failed to read GitHub App info
				}
			}
		}

		prUrl, err = gitClient.UndoPaCMergeRequest(repoUrl, mrData)
		return baseBranch, prUrl, "delete", err
	} else {
		// Close merge request.
		// To close a merge request it's enough to delete the branch.

		// Non-existing source branch should not be an error, just ignore it,
		// but other errors should be handled.
		if _, err := gitClient.DeleteBranch(repoUrl, sourceBranch); err != nil {
			return baseBranch, "", "", err
		}
		log.Info(fmt.Sprintf("pull request source branch %s is deleted", sourceBranch), l.Action, l.ActionDelete)
		return baseBranch, prUrl, "close", nil
	}
}

// generatePaCPipelineRunForComponent returns pipeline run definition to build component source with.
// Generated pipeline run contains placeholders that are expanded by Pipeline-as-Code.
func generatePaCPipelineRunForComponent(
	component *appstudiov1alpha1.Component,
	pipelineSpec *tektonapi.PipelineSpec,
	additionalPipelineParams []tektonapi.Param,
	onPull bool,
	pacTargetBranch string,
	gitClient gp.GitProviderClient) (*tektonapi.PipelineRun, error) {

	if pacTargetBranch == "" {
		return nil, fmt.Errorf("target branch can't be empty for generating PaC PipelineRun for: %v", component)
	}
	pipelineCelExpression, err := generateCelExpressionForPipeline(component, gitClient, pacTargetBranch, onPull)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cel expression for pipeline: %w", err)
	}
	repoUrl := component.Spec.Source.GitSource.URL

	annotations := map[string]string{
		"pipelinesascode.tekton.dev/max-keep-runs": "3",
		"build.appstudio.redhat.com/target_branch": "{{target_branch}}",
		pacCelExpressionAnnotationName:             pipelineCelExpression,
		gitCommitShaAnnotationName:                 "{{revision}}",
		gitRepoAtShaAnnotationName:                 gitClient.GetBrowseRepositoryAtShaLink(repoUrl, "{{revision}}"),
	}
	labels := map[string]string{
		ApplicationNameLabelName:                component.Spec.Application,
		ComponentNameLabelName:                  component.Name,
		"pipelines.appstudio.openshift.io/type": "build",
	}

	imageRepo := getContainerImageRepositoryForComponent(component)

	var pipelineName string
	var proposedImage string
	if onPull {
		annotations["build.appstudio.redhat.com/pull_request_number"] = "{{pull_request_number}}"
		pipelineName = component.Name + pipelineRunOnPRSuffix
		proposedImage = imageRepo + ":on-pr-{{revision}}"
	} else {
		pipelineName = component.Name + pipelineRunOnPushSuffix
		proposedImage = imageRepo + ":{{revision}}"
	}

	params := []tektonapi.Param{
		{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "{{repo_url}}"}},
		{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "{{revision}}"}},
		{Name: "output-image", Value: tektonapi.ParamValue{Type: "string", StringVal: proposedImage}},
	}
	if onPull {
		prImageExpiration := os.Getenv(PipelineRunOnPRExpirationEnvVar)
		if prImageExpiration == "" {
			prImageExpiration = PipelineRunOnPRExpirationDefault
		}
		params = append(params, tektonapi.Param{Name: "image-expires-after", Value: tektonapi.ParamValue{Type: "string", StringVal: prImageExpiration}})
	}

	dockerFile, err := DevfileSearchForDockerfile([]byte(component.Status.Devfile))
	if err != nil {
		return nil, boerrors.NewBuildOpError(boerrors.EInvalidDevfile, err)
	}
	if dockerFile != nil {
		if dockerFile.Uri != "" {
			params = append(params, tektonapi.Param{Name: "dockerfile", Value: tektonapi.ParamValue{Type: "string", StringVal: dockerFile.Uri}})
		}
		pathContext := getPathContext(component.Spec.Source.GitSource.Context, dockerFile.BuildContext)
		if pathContext != "" {
			params = append(params, tektonapi.Param{Name: "path-context", Value: tektonapi.ParamValue{Type: "string", StringVal: pathContext}})
		}
	}

	params = mergeAndSortTektonParams(params, additionalPipelineParams)

	pipelineRunWorkspaces := createWorkspaceBinding(pipelineSpec.Workspaces)

	pipelineRun := &tektonapi.PipelineRun{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PipelineRun",
			APIVersion: "tekton.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        pipelineName,
			Namespace:   component.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: tektonapi.PipelineRunSpec{
			PipelineSpec: pipelineSpec,
			Params:       params,
			Workspaces:   pipelineRunWorkspaces,
		},
	}

	return pipelineRun, nil
}

// generateCelExpressionForPipeline generates value for pipelinesascode.tekton.dev/on-cel-expression annotation
// in order to have better flexibility with git events filtering.
// Examples of returned values:
// event == "push" && target_branch == "main"
// event == "pull_request" && target_branch == "my-branch" && ( "component-src-dir/***".pathChanged() || "dockerfiles/my-component/Dockerfile".pathChanged() )
func generateCelExpressionForPipeline(component *appstudiov1alpha1.Component, gitClient gp.GitProviderClient, targetBranch string, onPull bool) (string, error) {
	eventType := "push"
	if onPull {
		eventType = "pull_request"
	}
	eventCondition := fmt.Sprintf(`event == "%s"`, eventType)

	targetBranchCondition := fmt.Sprintf(`target_branch == "%s"`, targetBranch)

	// Set path changed event filtering only for Components that are stored within a directory of the git repository.
	// Also, we have to rebuild everything on push events, so applying the filter only to pull request pipeline.
	pathChangedSuffix := ""
	if onPull && component.Spec.Source.GitSource.Context != "" && component.Spec.Source.GitSource.Context != "/" && component.Spec.Source.GitSource.Context != "./" && component.Spec.Source.GitSource.Context != "." {
		contextDir := component.Spec.Source.GitSource.Context
		if !strings.HasSuffix(contextDir, "/") {
			contextDir += "/"
		}

		// If a Dockerfile is defined for the Component,
		// we should rebuild the Component if the Dockerfile has been changed.
		dockerfilePathChangedSuffix := ""
		dockerfile, err := devfile.SearchForDockerfile([]byte(component.Status.Devfile))
		if err == nil && dockerfile != nil && dockerfile.Uri != "" {
			// Ignore dockerfile that is not stored in the same git repository but downloaded by an URL.
			if !strings.Contains(dockerfile.Uri, "://") {
				// dockerfile.Uri could be relative to the context directory or repository root.
				// To avoid unessesary builds, it's required to pass absolute path to the Dockerfile.
				repoUrl := component.Spec.Source.GitSource.URL
				branch := component.Spec.Source.GitSource.Revision
				dockerfilePath := contextDir + dockerfile.Uri
				isDockerfileInContextDir, err := gitClient.IsFileExist(repoUrl, branch, dockerfilePath)
				if err != nil {
					return "", err
				}
				// If the Dockerfile is inside context directory, no changes to event filter needed.
				if !isDockerfileInContextDir {
					// Pipelines as Code doesn't match path if it starts from /
					dockerfileAbsolutePath := strings.TrimPrefix(dockerfile.Uri, "/")
					dockerfilePathChangedSuffix = fmt.Sprintf(`|| "%s".pathChanged() `, dockerfileAbsolutePath)
				}
			}
		}

		pullPipelineFileName := component.Name + "-" + pipelineRunOnPRFilename
		pathChangedSuffix = fmt.Sprintf(` && ( "%s***".pathChanged() || ".tekton/%s".pathChanged() %s)`, contextDir, pullPipelineFileName, dockerfilePathChangedSuffix)
	}

	return fmt.Sprintf("%s && %s%s", eventCondition, targetBranchCondition, pathChangedSuffix), nil
}

func createWorkspaceBinding(pipelineWorkspaces []tektonapi.PipelineWorkspaceDeclaration) []tektonapi.WorkspaceBinding {
	pipelineRunWorkspaces := []tektonapi.WorkspaceBinding{}
	for _, workspace := range pipelineWorkspaces {
		switch workspace.Name {
		case "workspace":
			pipelineRunWorkspaces = append(pipelineRunWorkspaces,
				tektonapi.WorkspaceBinding{
					Name:                workspace.Name,
					VolumeClaimTemplate: generateVolumeClaimTemplate(),
				})
		case "git-auth":
			pipelineRunWorkspaces = append(pipelineRunWorkspaces,
				tektonapi.WorkspaceBinding{
					Name:   workspace.Name,
					Secret: &corev1.SecretVolumeSource{SecretName: "{{ git_auth_secret }}"},
				})
		}
	}
	return pipelineRunWorkspaces
}

// retrievePipelineSpec retrieves pipeline definition with given name from the given bundle.
func retrievePipelineSpec(ctx context.Context, bundleUri, pipelineName string) (*tektonapi.PipelineSpec, error) {
	log := ctrllog.FromContext(ctx)

	var obj runtime.Object
	var err error
	resolver := oci.NewResolver(bundleUri, authn.DefaultKeychain)

	if obj, _, err = resolver.Get(ctx, "pipeline", pipelineName); err != nil {
		return nil, err
	}

	var pipelineSpec tektonapi.PipelineSpec

	if v1beta1Pipeline, ok := obj.(tektonapi_v1beta1.PipelineObject); ok {
		v1beta1PipelineSpec := v1beta1Pipeline.PipelineSpec()
		metadata := v1beta1Pipeline.PipelineMetadata()
		log.Info("Converting from v1beta1 to v1", "PipelineName", pipelineName, "Bundle", bundleUri)
		err := v1beta1PipelineSpec.ConvertTo(ctx, &pipelineSpec, &metadata)
		if err != nil {
			return nil, boerrors.NewBuildOpError(
				boerrors.EPipelineConversionFailed,
				fmt.Errorf("pipeline %s from bundle %s: failed to convert from v1beta1 to v1: %w", pipelineName, bundleUri, err),
			)
		}
	} else if v1Pipeline, ok := obj.(*tektonapi.Pipeline); ok {
		pipelineSpec = v1Pipeline.PipelineSpec()
	} else {
		return nil, boerrors.NewBuildOpError(
			boerrors.EPipelineRetrievalFailed,
			fmt.Errorf("failed to extract pipeline %s from bundle %s", pipelineName, bundleUri),
		)
	}

	return &pipelineSpec, nil
}

// updateIncoming updates incomings in repository, adds new incoming for provided branch with incoming secret
// if repository contains multiple incoming entries, it will merge them to one, and combine Targets and add incoming secret to incoming
// if repository contains one incoming entry, it will add new target and add incoming secret to incoming
// if repository doesn't have any incoming entry, it will add new incoming entry with target and add incoming secret to incoming
// Returns bool, indicating if incomings in repository was updated or not
func updateIncoming(repository *pacv1alpha1.Repository, incomingSecretName string, pacIncomingSecretKey string, targetBranch string) bool {
	foundSecretName := false
	foundTarget := false
	multiple_incomings := false
	all_targets := []string{}

	if repository.Spec.Incomings != nil {
		if len(*repository.Spec.Incomings) > 1 {
			multiple_incomings = true
		}

		for idx, key := range *repository.Spec.Incomings {
			if multiple_incomings { // for multiple incomings gather all targets
				for _, target := range key.Targets {
					all_targets = append(all_targets, target)
					if target == targetBranch {
						foundTarget = true
					}
				}
			} else { // for single incoming add target & secret if missing
				for _, target := range key.Targets {
					if target == targetBranch {
						foundTarget = true
						break
					}
				}
				// add missing target branch
				if !foundTarget {
					(*repository.Spec.Incomings)[idx].Targets = append((*repository.Spec.Incomings)[idx].Targets, targetBranch)
				}

				if key.Secret.Name == incomingSecretName {
					foundSecretName = true
				} else {
					(*repository.Spec.Incomings)[idx].Secret = pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}
				}
			}
		}

		// combine multiple incomings into one and add secret
		if multiple_incomings {
			if !foundTarget {
				all_targets = append(all_targets, targetBranch)
			}
			incoming := []pacv1alpha1.Incoming{{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}, Targets: all_targets}}
			repository.Spec.Incomings = &incoming
		}
	} else {
		// create incomings when missing
		incoming := []pacv1alpha1.Incoming{{Type: "webhook-url", Secret: pacv1alpha1.Secret{Name: incomingSecretName, Key: pacIncomingSecretKey}, Targets: []string{targetBranch}}}
		repository.Spec.Incomings = &incoming
	}

	return multiple_incomings || !(foundSecretName && foundTarget)
}
