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

package v1alpha1

import (
	"encoding/json"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// WhenConditions defines requirements when specified build pipeline must be used.
// All conditions are connected via AND, whereas cases within any condition connected via OR.
// Example:
//
//	language: java
//	projectType: spring,quarkus
//	annotations:
//	   builder: gradle,maven
//
// which means that language is 'java' AND (project type is 'spring' OR 'quarkus') AND
// annotation 'builder' is present with value 'gradle' OR 'maven'.
type WhenCondition struct {
	// Defines component language to match, e.g. 'java'.
	// The value to compare with is taken from devfile.metadata.language field.
	// +kubebuilder:validation:Optional
	Language string `json:"language,omitempty"`

	// Defines type of project of the component to match, e.g. 'quarkus'.
	// The value to compare with is taken from devfile.metadata.projectType field.
	// +kubebuilder:validation:Optional
	ProjectType string `json:"projectType,omitempty"`

	// Defines if a Dockerfile should be present in the component.
	// Note, unset (nil) value is not the same as false (unset means skip the dockerfile check).
	// The value to compare with is taken from devfile components of image type.
	// +kubebuilder:validation:Optional
	DockerfileRequired *bool `json:"dockerfile,omitempty"`

	// Defines list of allowed component names to match, e.g. 'my-component'.
	// The value to compare with is taken from component.metadata.name field.
	// +kubebuilder:validation:Optional
	ComponentName string `json:"componentName,omitempty"`

	// Defines annotations to match.
	// The values to compare with are taken from component.metadata.annotations field.
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Defines labels to match.
	// The values to compare with are taken from component.metadata.labels field.
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`
}

// PipelineParam is a type to describe pipeline parameters.
// tektonapi.Param type is not used due to validation issues.
type PipelineParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PipelineSelector defines allowed build pipeline and conditions when it should be used.
type PipelineSelector struct {
	// Name of the selector item. Optional.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Build Pipeline to use if the selector conditions are met.
	// +kubebuilder:validation:Required
	PipelineRef PipelineRef `json:"pipelineRef"`

	// Extra arguments to add to the specified pipeline run.
	// +kubebuilder:validation:Optional
	// +listType=atomic
	PipelineParams []PipelineParam `json:"pipelineParams,omitempty"`

	// Defines the selector conditions when given build pipeline should be used.
	// All conditions are connected via AND, whereas cases within any condition connected via OR.
	// If the section is omitted, then the condition is considered true (usually used for fallback condition).
	// +kubebuilder:validation:Optional
	WhenConditions WhenCondition `json:"when,omitempty"`
}

// PipelineRef can be used to refer to a specific instance of a Pipeline.
type PipelineRef struct {
	// Name of the referent; More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name,omitempty"`
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// ResolverRef allows referencing a Pipeline in a remote location
	// like a git repo. This field is only supported when the alpha
	// feature gate is enabled.
	// +optional
	ResolverRef `json:",omitempty"`
}

func (r PipelineRef) ConvertToTekton() tektonapi.PipelineRef {
	return tektonapi.PipelineRef{
		Name:        r.Name,
		APIVersion:  r.APIVersion,
		ResolverRef: r.ResolverRef.ConvertToTekton(),
	}
}

// ResolverRef can be used to refer to a Pipeline or Task in a remote
// location like a git repo. This feature is in beta and these fields
// are only available when the beta feature gate is enabled.
type ResolverRef struct {
	// Resolver is the name of the resolver that should perform
	// resolution of the referenced Tekton resource, such as "git".
	// +optional
	Resolver tektonapi.ResolverName `json:"resolver,omitempty"`
	// Params contains the parameters used to identify the
	// referenced Tekton resource. Example entries might include
	// "repo" or "path" but the set of params ultimately depends on
	// the chosen resolver.
	// +optional
	// +listType=atomic
	Params []Param `json:"params,omitempty"`
}

func (r ResolverRef) ConvertToTekton() tektonapi.ResolverRef {

	params := []tektonapi.Param{}
	for _, p := range r.Params {
		params = append(params, p.ConvertToTekton())
	}
	return tektonapi.ResolverRef{
		Resolver: r.Resolver,
		Params:   params,
	}
}

// Param declares an ParamValues to use for the parameter called name.
type Param struct {
	Name  string     `json:"name"`
	Value ParamValue `json:"value"`
}

func (r Param) ConvertToTekton() tektonapi.Param {
	return tektonapi.Param{
		Name:  r.Name,
		Value: r.Value.ConvertToTekton(),
	}
}

// ParamValue is a type that can hold a single string, string array, or string map.
// Used in JSON unmarshalling so that a single JSON field can accept
// either an individual string or an array of strings.
type ParamValue struct {
	Type      tektonapi.ParamType `json:"type"` // Represents the stored type of ParamValues.
	StringVal string              `json:"stringVal"`
	// +listType=atomic
	ArrayVal  []string          `json:"arrayVal"`
	ObjectVal map[string]string `json:"objectVal"`
}

func (r ParamValue) ConvertToTekton() tektonapi.ParamValue {
	return tektonapi.ParamValue{
		Type:      r.Type,
		StringVal: r.StringVal,
		ArrayVal:  r.ArrayVal,
		ObjectVal: r.ObjectVal,
	}
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (paramValues *ParamValue) UnmarshalJSON(value []byte) error {
	// ParamValues is used for Results Value as well, the results can be any kind of
	// data so we need to check if it is empty.
	if len(value) == 0 {
		paramValues.Type = tektonapi.ParamTypeString
		return nil
	}
	if value[0] == '[' {
		// We're trying to Unmarshal to []string, but for cases like []int or other types
		// of nested array which we don't support yet, we should continue and Unmarshal
		// it to String. If the Type being set doesn't match what it actually should be,
		// it will be captured by validation in reconciler.
		// if failed to unmarshal to array, we will convert the value to string and marshal it to string
		var a []string
		if err := json.Unmarshal(value, &a); err == nil {
			paramValues.Type = tektonapi.ParamTypeArray
			paramValues.ArrayVal = a
			return nil
		}
	}
	if value[0] == '{' {
		// if failed to unmarshal to map, we will convert the value to string and marshal it to string
		var m map[string]string
		if err := json.Unmarshal(value, &m); err == nil {
			paramValues.Type = tektonapi.ParamTypeObject
			paramValues.ObjectVal = m
			return nil
		}
	}

	// By default we unmarshal to string
	paramValues.Type = tektonapi.ParamTypeString
	if err := json.Unmarshal(value, &paramValues.StringVal); err == nil {
		return nil
	}
	paramValues.StringVal = string(value)

	return nil
}

// MarshalJSON implements the json.Marshaller interface.
func (paramValues ParamValue) MarshalJSON() ([]byte, error) {
	switch paramValues.Type {
	case tektonapi.ParamTypeString:
		return json.Marshal(paramValues.StringVal)
	case tektonapi.ParamTypeArray:
		return json.Marshal(paramValues.ArrayVal)
	case tektonapi.ParamTypeObject:
		return json.Marshal(paramValues.ObjectVal)
	default:
		return []byte{}, fmt.Errorf("impossible ParamValues.Type: %q", paramValues.Type)
	}
}

// NewStructuredValues creates an ParamValues of type ParamTypeString or ParamTypeArray, based on
// how many inputs are given (>1 input will create an array, not string).
func NewStructuredValues(value string, values ...string) *ParamValue {
	if len(values) > 0 {
		return &ParamValue{
			Type:     tektonapi.ParamTypeArray,
			ArrayVal: append([]string{value}, values...),
		}
	}
	return &ParamValue{
		Type:      tektonapi.ParamTypeString,
		StringVal: value,
	}
}

// BuildPipelineSelectorSpec defines the desired state of BuildPipelineSelector
type BuildPipelineSelectorSpec struct {
	// Defines chain of pipeline selectors.
	// The first matching item is used.
	// +kubebuilder:validation:Required
	Selectors []PipelineSelector `json:"selectors"`
}

//+kubebuilder:object:root=true

// BuildPipelineSelector is the Schema for the BuildPipelineSelectors API
type BuildPipelineSelector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BuildPipelineSelectorSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// BuildPipelineSelectorList contains a list of BuildPipelineSelector
type BuildPipelineSelectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildPipelineSelector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildPipelineSelector{}, &BuildPipelineSelectorList{})
}
