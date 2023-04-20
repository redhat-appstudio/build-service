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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	buildappstudiov1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	pipelineselector "github.com/redhat-appstudio/build-service/pkg/pipeline-selector"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

// GetPipelineForComponent searches for the build pipeline to use on the component.
func (r *ComponentBuildReconciler) GetPipelineForComponent(ctx context.Context, component *appstudiov1alpha1.Component) (*tektonapi.PipelineRef, []tektonapi.Param, error) {
	var pipelineSelectors []buildappstudiov1alpha1.BuildPipelineSelector
	pipelineSelector := &buildappstudiov1alpha1.BuildPipelineSelector{}

	pipelineSelectorKeys := []types.NamespacedName{
		// First try specific config for the application
		{Namespace: component.Namespace, Name: component.Spec.Application},
		// Second try namespaced config
		{Namespace: component.Namespace, Name: buildPipelineSelectorResourceName},
		// Finally try global config
		{Namespace: buildServiceNamespaceName, Name: buildPipelineSelectorResourceName},
	}

	for _, pipelineSelectorKey := range pipelineSelectorKeys {
		if err := r.Client.Get(ctx, pipelineSelectorKey, pipelineSelector); err != nil {
			if !errors.IsNotFound(err) {
				return nil, nil, err
			}
			// The config is not found, try the next one in the hierarchy
		} else {
			pipelineSelectors = append(pipelineSelectors, *pipelineSelector)
		}
	}

	if len(pipelineSelectors) > 0 {
		pipelineRef, pipelineParams, err := pipelineselector.SelectPipelineForComponent(component, pipelineSelectors)
		if err != nil {
			return nil, nil, err
		}
		if pipelineRef != nil {
			return pipelineRef, pipelineParams, nil
		}
	}

	// Fallback to the default pipeline
	return &tektonapi.PipelineRef{
		Name:   defaultPipelineName,
		Bundle: defaultPipelineBundle,
	}, nil, nil
}

func (r *ComponentBuildReconciler) linkSecretToServiceAccount(ctx context.Context, secretName, serviceAccountName, namespace string, isPullSecret bool) (bool, error) {
	log := r.Log

	if secretName == "" {
		// The secret is empty, no updates needed
		return false, nil
	}

	serviceAccount := &corev1.ServiceAccount{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: serviceAccountName, Namespace: namespace}, serviceAccount)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.Log.Error(err, fmt.Sprintf("Failed to read service account %s in namespace %s", serviceAccountName, namespace))
		return false, err
	}

	isNewSecretLinked := false

	shouldAddLink := true
	for _, secret := range serviceAccount.Secrets {
		if secret.Name == secretName {
			// The secret is present in the service account, no updates needed
			shouldAddLink = false
			break
		}
	}
	if shouldAddLink {
		serviceAccount.Secrets = append(serviceAccount.Secrets, corev1.ObjectReference{Name: secretName, Namespace: namespace})
	}
	isNewSecretLinked = shouldAddLink

	if isPullSecret {
		shouldAddLink = true
		for _, secret := range serviceAccount.ImagePullSecrets {
			if secret.Name == secretName {
				// The secret is present in the service account, no updates needed
				shouldAddLink = false
				break
			}
		}
		if shouldAddLink {
			serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, corev1.LocalObjectReference{Name: secretName})
		}
		isNewSecretLinked = isNewSecretLinked || shouldAddLink
	}

	// Update service account if needed
	if isNewSecretLinked {
		err := r.Client.Update(ctx, serviceAccount)
		if err != nil {
			log.Error(err, fmt.Sprintf("Unable to update service account %s", serviceAccount.Name))
			return false, err
		}
		log.Info(fmt.Sprintf("Service Account %s updated with secret %s", serviceAccount.Name, secretName))
		return true, nil
	}
	return false, nil
}

func getContainerImageRepositoryForComponent(component *appstudiov1alpha1.Component) string {
	if component.Spec.ContainerImage != "" {
		return getContainerImageRepository(component.Spec.ContainerImage)
	}
	imageRepo, _, err := getComponentImageRepoAndSecretNameFromImageAnnotation(component)
	if err == nil && imageRepo != "" {
		return imageRepo
	}
	return ""
}

// getContainerImageRepository removes tag or SHA has from container image reference
func getContainerImageRepository(image string) string {
	if strings.Contains(image, "@") {
		// registry.io/user/image@sha256:586ab...d59a
		return strings.Split(image, "@")[0]
	}
	// registry.io/user/image:tag
	return strings.Split(image, ":")[0]
}

// getComponentImageRepoAndSecretNameFromImageAnnotation parses image.redhat.com/image annotation
// for image repository and secret name to access it.
// If image.redhat.com/generate is not set, the procedure returns empty values.
func getComponentImageRepoAndSecretNameFromImageAnnotation(component *appstudiov1alpha1.Component) (string, string, error) {
	type RepositoryInfo struct {
		Image  string `json:"image"`
		Secret string `json:"secret"`
	}

	var repoInfo RepositoryInfo
	if imageRepoDataJson, exists := component.Annotations[ImageRepoAnnotationName]; exists {
		if err := json.Unmarshal([]byte(imageRepoDataJson), &repoInfo); err != nil {
			return "", "", boerrors.NewBuildOpError(boerrors.EFailedToParseImageAnnotation, err)
		}
		return repoInfo.Image, repoInfo.Secret, nil
	}
	return "", "", nil
}

// mergeAndSortTektonParams merges additional params into existing params by adding new or replacing existing values.
func mergeAndSortTektonParams(existedParams, additionalParams []tektonapi.Param) []tektonapi.Param {
	var params []tektonapi.Param
	paramsMap := make(map[string]tektonapi.Param)
	for _, p := range existedParams {
		paramsMap[p.Name] = p
	}
	for _, p := range additionalParams {
		paramsMap[p.Name] = p
	}
	for _, v := range paramsMap {
		params = append(params, v)
	}
	sort.Slice(params, func(i, j int) bool {
		return params[i].Name < params[j].Name
	})
	return params
}

func generateVolumeClaimTemplate() *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				"ReadWriteOnce",
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"storage": resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func getPathContext(gitContext, dockerfileContext string) string {
	if gitContext == "" && dockerfileContext == "" {
		return ""
	}
	separator := string(filepath.Separator)
	path := filepath.Join(gitContext, dockerfileContext)
	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, separator)
	return path
}
