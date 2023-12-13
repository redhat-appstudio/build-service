/*
Copyright 2022.

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

package pipelineselector

import (
	"strings"

	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	devfile "github.com/redhat-appstudio/application-service/pkg/devfile"
	buildappstudiov1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
)

// SelectPipelineForComponent evaluates given list of pipeline selectors against specified component
// to find the build pipeline for the component.
// The first match is returned.
func SelectPipelineForComponent(component *appstudiov1alpha1.Component, selectors []buildappstudiov1alpha1.BuildPipelineSelector) (*tektonapi.PipelineRef, []tektonapi.Param, error) {
	selectionParameters, err := getPipelineSelectionParametersForComponent(component)
	if err != nil {
		return nil, nil, err
	}

	for i := range selectors {
		if buildPipelineRef, buildPipelineAdditionalParams := findMatchingPipeline(selectionParameters, &selectors[i]); buildPipelineRef != nil {
			return buildPipelineRef, buildPipelineAdditionalParams, nil
		}
	}
	return nil, nil, nil
}

// getPipelineSelectionParametersForComponent returns build parameters of the given component
// to use when matching to the build pipelines selection logic.
func getPipelineSelectionParametersForComponent(component *appstudiov1alpha1.Component) (*buildappstudiov1alpha1.WhenCondition, error) {
	parameters := &buildappstudiov1alpha1.WhenCondition{}

	parameters.ComponentName = component.GetName()
	parameters.Annotations = component.GetAnnotations()
	parameters.Labels = component.GetLabels()
	devfileSrc := devfile.DevfileSrc{
		Data: component.Status.Devfile,
	}

	devfileData, err := devfile.ParseDevfile(devfileSrc)
	if err != nil {
		return nil, boerrors.NewBuildOpError(boerrors.EInvalidDevfile, err)
	}

	devfileMetadata := devfileData.GetMetadata()

	parameters.Language = devfileMetadata.Language
	parameters.ProjectType = devfileMetadata.ProjectType

	var dockerfileRequired bool
	dockerfile, err := devfile.SearchForDockerfile([]byte(component.Status.Devfile))
	if err != nil {
		return nil, boerrors.NewBuildOpError(boerrors.EInvalidDevfile, err)
	}
	if dockerfile != nil {
		dockerfileRequired = true
		parameters.DockerfileRequired = &dockerfileRequired
	} else {
		dockerfileRequired = false
		parameters.DockerfileRequired = &dockerfileRequired
	}

	return parameters, nil
}

// findMatchingPipeline evaluates given selectors chain against component parameters.
// The first match is returned.
func findMatchingPipeline(selectionParameters *buildappstudiov1alpha1.WhenCondition, selectors *buildappstudiov1alpha1.BuildPipelineSelector) (*tektonapi.PipelineRef, []tektonapi.Param) {
	for _, pipelineSelector := range selectors.Spec.Selectors {
		if pipelineConditionsMatchComponentParameters(&pipelineSelector.WhenConditions, selectionParameters) {
			var pipelineParams []tektonapi.Param
			for _, param := range pipelineSelector.PipelineParams {
				pipelineParams = append(pipelineParams, tektonapi.Param{
					Name:  param.Name,
					Value: *tektonapi.NewStructuredValues(param.Value),
				})
			}
			pipelineRef := pipelineSelector.PipelineRef.ConvertToTekton()
			return &pipelineRef, pipelineParams
		}
	}
	return nil, nil
}

// pipelineConditionsMatchComponentParameters evaluates given pipeline selector against component parameters.
// In other words, checks if given pipeline can build the component (according to what the pipeline conditions say).
func pipelineConditionsMatchComponentParameters(pipeline, component *buildappstudiov1alpha1.WhenCondition) bool {
	if pipeline.Language != "" && !pipelineMatchesComponentCondition(pipeline.Language, component.Language) {
		return false
	}
	if pipeline.ProjectType != "" && !pipelineMatchesComponentCondition(pipeline.ProjectType, component.ProjectType) {
		return false
	}

	if pipeline.DockerfileRequired != nil && *pipeline.DockerfileRequired != *component.DockerfileRequired {
		return false
	}

	if pipeline.ComponentName != "" && !pipelineMatchesComponentCondition(pipeline.ComponentName, component.ComponentName) {
		return false
	}

	if len(pipeline.Labels) != 0 && !pipelineMatchesComponentLabels(pipeline.Labels, component.Labels) {
		return false
	}
	if len(pipeline.Annotations) != 0 && !pipelineMatchesComponentLabels(pipeline.Annotations, component.Annotations) {
		return false
	}

	return true
}

// pipelineMatchesComponentCondition checks if component condition is covered by the pipeline conditions.
// For example, component condition (for the language key) is "java", pipeline conditipons are "python,java,nodejs", result is true.
func pipelineMatchesComponentCondition(pipelineConditions, componentCondition string) bool {
	componentCondition = strings.ToLower(strings.TrimSpace(componentCondition))

	values := strings.Split(pipelineConditions, ",")
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if componentCondition == value {
			return true
		}
	}

	return false
}

// pipelineMatchesComponentLabels checks if given pipeline supports build of the the component by looking at labels.
// For example, component labels are:
//
//	appstudio/builder: maven
//	anotherLable: someValue
//
// required by the pipeline labels are:
//
//	appstudio/builder: maven,gradle
//
// The result is true.
func pipelineMatchesComponentLabels(pipelineLabels, componentLabels map[string]string) bool {
	for labelName, labelSupportedValues := range pipelineLabels {
		if componentLabelValue, componentLabelExists := componentLabels[labelName]; componentLabelExists {
			if !pipelineMatchesComponentCondition(labelSupportedValues, componentLabelValue) {
				return false
			}
		} else {
			return false
		}
	}

	return true
}
