/*
Copyright 2021-2022 Red Hat, Inc.

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
	"fmt"
	"github.com/redhat-appstudio/build-service/pkg/bometrics"
	"reflect"
	"strings"
	"testing"

	"github.com/devfile/api/v2/pkg/apis/workspaces/v1alpha2"
	devfile "github.com/redhat-appstudio/application-service/cdq-analysis/pkg"
	"github.com/redhat-appstudio/application-service/gitops"
	"github.com/redhat-appstudio/build-service/pkg/boerrors"
	"gotest.tools/v3/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

const ghAppPrivateKeyStub = "-----BEGIN RSA PRIVATE KEY-----_key-content_-----END RSA PRIVATE KEY-----"

func TestGetProvisionTimeMetricsBuckets(t *testing.T) {
	buckets := bometrics.HistogramBuckets
	for i := 1; i < len(buckets); i++ {
		if buckets[i] <= buckets[i-1] {
			t.Errorf("Buckets must be in increasing order, but got: %v", buckets)
		}
	}
}

func TestReadBuildStatus(t *testing.T) {
	tests := []struct {
		name                       string
		buildStatusAnnotationValue string
		want                       *BuildStatus
	}{
		{
			name:                       "should be able to read build status with all fields",
			buildStatusAnnotationValue: "{\"simple\":{\"build-start-time\":\"time\",\"error-id\":1,\"error-message\":\"simple-build-error\"},\"pac\":{\"state\":\"enabled\",\"configuration-time\":\"time\",\"error-id\":5,\"error-message\":\"pac-error\"},\"message\":\"done\"}",
			want: &BuildStatus{
				Simple: &SimpleBuildStatus{
					BuildStartTime: "time",
					ErrorInfo: ErrorInfo{
						ErrId:      1,
						ErrMessage: "simple-build-error",
					},
				},
				PaC: &PaCBuildStatus{
					State:             "enabled",
					ConfigurationTime: "time",
					ErrorInfo: ErrorInfo{
						ErrId:      5,
						ErrMessage: "pac-error",
					},
				},
				Message: "done",
			},
		},
		{
			name:                       "should return empty build status if annotation is empty or not set",
			buildStatusAnnotationValue: "",
			want:                       &BuildStatus{},
		},
		{
			name:                       "should return empty build status if curent one is not valid JSON",
			buildStatusAnnotationValue: "{\"simple\":{\"build-start-time\":\"time\"}",
			want:                       &BuildStatus{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component := &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-component",
					Namespace: "my-namespace",
					Annotations: map[string]string{
						BuildStatusAnnotationName: tt.buildStatusAnnotationValue,
					},
				},
			}
			got := readBuildStatus(component)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readBuildStatus(): actual: %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteBuildStatus(t *testing.T) {
	tests := []struct {
		name        string
		component   *appstudiov1alpha1.Component
		buildStatus *BuildStatus
		want        string
	}{
		{
			name: "should be able to write build status with all fields",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "my-component",
					Namespace:   "my-namespace",
					Annotations: map[string]string{},
				},
			},
			buildStatus: &BuildStatus{
				Simple: &SimpleBuildStatus{
					BuildStartTime: "time",
					ErrorInfo: ErrorInfo{
						ErrId:      1,
						ErrMessage: "simple-build-error",
					},
				},
				PaC: &PaCBuildStatus{
					State: "enabled",
					ErrorInfo: ErrorInfo{
						ErrId:      5,
						ErrMessage: "pac-error",
					},
					ConfigurationTime: "time",
				},
				Message: "done",
			},
			want: "{\"simple\":{\"build-start-time\":\"time\",\"error-id\":1,\"error-message\":\"simple-build-error\"},\"pac\":{\"state\":\"enabled\",\"configuration-time\":\"time\",\"error-id\":5,\"error-message\":\"pac-error\"},\"message\":\"done\"}",
		},
		{
			name: "should be able to write build status when annotations is nil",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-component",
					Namespace: "my-namespace",
				},
			},
			buildStatus: &BuildStatus{
				Simple: &SimpleBuildStatus{
					BuildStartTime: "time",
				},
				Message: "done",
			},
			want: "{\"simple\":{\"build-start-time\":\"time\"},\"message\":\"done\"}",
		},
		{
			name: "should be able to overwrite build status",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-component",
					Namespace: "my-namespace",
					Annotations: map[string]string{
						BuildStatusAnnotationName: "{\"simple\":{\"build-start-time\":\"time\"},\"message\":\"done\"}",
					},
				},
			},
			buildStatus: &BuildStatus{
				PaC: &PaCBuildStatus{
					State: "error",
					ErrorInfo: ErrorInfo{
						ErrId:      10,
						ErrMessage: "error-ion-pac",
					},
					ConfigurationTime: "time",
				},
				Message: "done",
			},
			want: "{\"pac\":{\"state\":\"error\",\"configuration-time\":\"time\",\"error-id\":10,\"error-message\":\"error-ion-pac\"},\"message\":\"done\"}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeBuildStatus(tt.component, tt.buildStatus)
			got := tt.component.Annotations[BuildStatusAnnotationName]
			if got != tt.want {
				t.Errorf("writeBuildStatus(): actual: %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateInitialPipelineRunForComponentDevfileError(t *testing.T) {
	component := &appstudiov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-component",
			Namespace: "my-namespace",
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			Application:    "my-application",
			ContainerImage: "registry.io/username/image:tag",
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL:      "https://githost.com/user/repo.git",
						Revision: "custom-branch",
					},
				},
			},
		},
		Status: appstudiov1alpha1.ComponentStatus{
			Devfile: "wrong",
		},
	}
	pipelineRef := &tektonapi.PipelineRef{
		ResolverRef: tektonapi.ResolverRef{
			Resolver: "bundles",
			Params: []tektonapi.Param{
				{Name: "name", Value: *tektonapi.NewStructuredValues("pipeline-name")},
				{Name: "bundle", Value: *tektonapi.NewStructuredValues("pipeline-bundle")},
			},
		},
	}
	additionalParams := []tektonapi.Param{
		{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "2378a064bf6b66a8ffc650ad88d404cca24ade29"}},
		{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
	}
	commitSha := "26239c94569cea79b32bce32f12c8abd8bbd0fd7"

	pRunGitInfo := &buildGitInfo{
		gitSourceSha:              commitSha,
		browseRepositoryAtShaLink: "https://githost.com/user/repo?rev=" + commitSha,
	}

	_, err := generatePipelineRunForComponent(component, pipelineRef, additionalParams, pRunGitInfo)
	if err == nil {
		t.Error("generateInitialPipelineRunForComponentDevfileError(): Didn't return error")
	} else {
		assert.ErrorContains(t, err, "invalid devfile due to error parsing devfile because of non-compliant data due to json")
	}
}

func TestGenerateInitialPipelineRunForComponentDockerfileContext(t *testing.T) {
	component := &appstudiov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-component",
			Namespace: "my-namespace",
			Annotations: map[string]string{
				"skip-initial-checks":            "true",
				gitops.GitProviderAnnotationName: "github",
			},
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			Application:    "my-application",
			ContainerImage: "registry.io/username/image:tag",
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL:      "https://githost.com/user/repo.git",
						Revision: "custom-branch",
					},
				},
			},
		},
		Status: appstudiov1alpha1.ComponentStatus{},
	}
	pipelineRef := &tektonapi.PipelineRef{
		ResolverRef: tektonapi.ResolverRef{
			Resolver: "bundles",
			Params: []tektonapi.Param{
				{Name: "name", Value: *tektonapi.NewStructuredValues("pipeline-name")},
				{Name: "bundle", Value: *tektonapi.NewStructuredValues("pipeline-bundle")},
			},
		},
	}
	additionalParams := []tektonapi.Param{
		{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
	}

	dockerfileContext := "some_context"
	dockerfileURI := "some_uri"
	DevfileSearchForDockerfile = func(devfileBytes []byte) (*v1alpha2.DockerfileImage, error) {
		dockerfileImage := v1alpha2.DockerfileImage{
			BaseImage:     v1alpha2.BaseImage{},
			DockerfileSrc: v1alpha2.DockerfileSrc{Uri: dockerfileURI},
			Dockerfile:    v1alpha2.Dockerfile{BuildContext: dockerfileContext},
		}
		return &dockerfileImage, nil
	}

	pipelineRun, err := generatePipelineRunForComponent(component, pipelineRef, additionalParams, &buildGitInfo{})

	if err != nil {
		t.Error("generateInitialPipelineRunForComponentDockerfileContext(): Failed to generate pipeline run")
	}

	for _, param := range pipelineRun.Spec.Params {
		switch param.Name {
		case "path-context":
			if param.Value.StringVal != dockerfileContext {
				t.Errorf("generateInitialPipelineRunForComponentDockerfileContext(): wrong pipeline parameter %s", param.Name)
			}
		case "dockerfile":
			if param.Value.StringVal != dockerfileURI {
				t.Errorf("generateInitialPipelineRunForComponentDockerfileContext(): wrong pipeline parameter %s", param.Name)
			}
		case "revision":
			if param.Value.StringVal != "custom-branch" {
				t.Errorf("generateInitialPipelineRunForComponentDockerfileContext(): wrong pipeline parameter %s", param.Name)
			}
		}
	}
}

func TestGenerateInitialPipelineRunForComponent(t *testing.T) {
	component := &appstudiov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-component",
			Namespace: "my-namespace",
			Annotations: map[string]string{
				"skip-initial-checks":            "true",
				gitops.GitProviderAnnotationName: "github",
			},
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			Application:    "my-application",
			ContainerImage: "registry.io/username/image:tag",
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL:      "https://githost.com/user/repo.git",
						Revision: "custom-branch",
					},
				},
			},
		},
		Status: appstudiov1alpha1.ComponentStatus{
			Devfile: getMinimalDevfile(),
		},
	}
	pipelineRef := &tektonapi.PipelineRef{
		ResolverRef: tektonapi.ResolverRef{
			Resolver: "bundles",
			Params: []tektonapi.Param{
				{Name: "name", Value: *tektonapi.NewStructuredValues("pipeline-name")},
				{Name: "bundle", Value: *tektonapi.NewStructuredValues("pipeline-bundle")},
			},
		},
	}
	additionalParams := []tektonapi.Param{
		{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
	}
	commitSha := "26239c94569cea79b32bce32f12c8abd8bbd0fd7"

	pRunGitInfo := &buildGitInfo{
		gitSourceSha:              commitSha,
		browseRepositoryAtShaLink: "https://githost.com/user/repo?rev=" + commitSha,
	}
	DevfileSearchForDockerfile = devfile.SearchForDockerfile
	pipelineRun, err := generatePipelineRunForComponent(component, pipelineRef, additionalParams, pRunGitInfo)
	if err != nil {
		t.Error("generateInitialPipelineRunForComponent(): Failed to genertate pipeline run")
	}

	if pipelineRun.GenerateName == "" {
		t.Error("generateInitialPipelineRunForComponent(): pipeline generatename must not be empty")
	}
	if pipelineRun.Namespace != "my-namespace" {
		t.Error("generateInitialPipelineRunForComponent(): pipeline namespace doesn't match")
	}

	if pipelineRun.Labels[ApplicationNameLabelName] != "my-application" {
		t.Errorf("generateInitialPipelineRunForComponent(): wrong %s label value", ApplicationNameLabelName)
	}
	if pipelineRun.Labels[ComponentNameLabelName] != "my-component" {
		t.Errorf("generateInitialPipelineRunForComponent(): wrong %s label value", ComponentNameLabelName)
	}
	if pipelineRun.Labels["pipelines.appstudio.openshift.io/type"] != "build" {
		t.Error("generateInitialPipelineRunForComponent(): wrong pipelines.appstudio.openshift.io/type label value")
	}

	if pipelineRun.Annotations["build.appstudio.redhat.com/target_branch"] != "custom-branch" {
		t.Error("generateInitialPipelineRunForComponent(): wrong build.appstudio.redhat.com/target_branch annotation value")
	}
	if pipelineRun.Annotations["build.appstudio.redhat.com/pipeline_name"] != "pipeline-name" {
		t.Error("generateInitialPipelineRunForComponent(): wrong build.appstudio.redhat.com/pipeline_name annotation value")
	}
	if pipelineRun.Annotations["build.appstudio.redhat.com/bundle"] != "pipeline-bundle" {
		t.Error("generateInitialPipelineRunForComponent(): wrong build.appstudio.redhat.com/bundle annotation value")
	}
	if pipelineRun.Annotations[gitCommitShaAnnotationName] != commitSha {
		t.Errorf("generateInitialPipelineRunForComponent(): wrong %s annotation value", gitCommitShaAnnotationName)
	}
	if pipelineRun.Annotations[gitRepoAtShaAnnotationName] != "https://githost.com/user/repo?rev="+commitSha {
		t.Errorf("generateInitialPipelineRunForComponent(): wrong %s annotation value", gitRepoAtShaAnnotationName)
	}

	if getPipelineName(pipelineRun.Spec.PipelineRef) != "pipeline-name" {
		t.Error("generateInitialPipelineRunForComponent(): wrong pipeline name in pipeline reference")
	}
	if getPipelineBundle(pipelineRun.Spec.PipelineRef) != "pipeline-bundle" {
		t.Error("generateInitialPipelineRunForComponent(): wrong pipeline bundle in pipeline reference")
	}

	if len(pipelineRun.Spec.Params) != 5 {
		t.Error("generateInitialPipelineRunForComponent(): wrong number of pipeline params")
	}
	for _, param := range pipelineRun.Spec.Params {
		switch param.Name {
		case "git-url":
			if param.Value.StringVal != "https://githost.com/user/repo.git" {
				t.Errorf("generateInitialPipelineRunForComponent(): wrong pipeline parameter %s", param.Name)
			}
		case "revision":
			if param.Value.StringVal != commitSha {
				t.Errorf("generateInitialPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "output-image":
			if !strings.HasPrefix(param.Value.StringVal, "registry.io/username/image:build-") {
				t.Errorf("generateInitialPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "rebuild":
			if param.Value.StringVal != "true" {
				t.Errorf("generateInitialPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "skip-checks":
			if param.Value.StringVal != "true" {
				t.Errorf("generateInitialPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		default:
			t.Errorf("generateInitialPipelineRunForComponent(): unexpected pipeline parameter %v", param)
		}
	}

	if len(pipelineRun.Spec.Workspaces) != 1 {
		t.Error("generateInitialPipelineRunForComponent(): wrong number of pipeline workspaces")
	}
	for _, workspace := range pipelineRun.Spec.Workspaces {
		if workspace.Name == "workspace" {
			continue
		}
		t.Errorf("generateInitialPipelineRunForComponent(): unexpected pipeline workspaces %v", workspace)
	}
}

func TestGeneratePaCPipelineRunForComponent(t *testing.T) {
	component := &appstudiov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-component",
			Namespace: "my-namespace",
			Annotations: map[string]string{
				"skip-initial-checks":            "true",
				gitops.GitProviderAnnotationName: "github",
			},
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			Application:    "my-application",
			ContainerImage: "registry.io/username/image:tag",
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL: "https://githost.com/user/repo.git",
					},
				},
			},
		},
		Status: appstudiov1alpha1.ComponentStatus{
			Devfile: getMinimalDevfile(),
		},
	}
	pipelineSpec := &tektonapi.PipelineSpec{
		Workspaces: []tektonapi.PipelineWorkspaceDeclaration{
			{
				Name: "git-auth",
			},
			{
				Name: "workspace",
			},
		},
	}
	additionalParams := []tektonapi.Param{
		{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "2378a064bf6b66a8ffc650ad88d404cca24ade29"}},
		{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
	}
	dockerfileURI := "dockerfile"
	dockerfileContext := "docker"
	DevfileSearchForDockerfile = func(devfileBytes []byte) (*v1alpha2.DockerfileImage, error) {
		dockerfileImage := v1alpha2.DockerfileImage{
			DockerfileSrc: v1alpha2.DockerfileSrc{Uri: dockerfileURI},
			Dockerfile:    v1alpha2.Dockerfile{BuildContext: dockerfileContext},
		}
		return &dockerfileImage, nil
	}
	branchName := "custom-branch"
	ResetTestGitProviderClient()

	pipelineRun, err := generatePaCPipelineRunForComponent(component, pipelineSpec, additionalParams, true, branchName, testGitProviderClient)
	if err != nil {
		t.Error("generatePaCPipelineRunForComponent(): Failed to genertate pipeline run")
	}

	if pipelineRun.Name != component.Name+pipelineRunOnPRSuffix {
		t.Error("generatePaCPipelineRunForComponent(): wrong pipeline name")
	}
	if pipelineRun.Namespace != "my-namespace" {
		t.Error("generatePaCPipelineRunForComponent(): pipeline namespace doesn't match")
	}

	if pipelineRun.Labels[ApplicationNameLabelName] != "my-application" {
		t.Errorf("generatePaCPipelineRunForComponent(): wrong %s label value", ApplicationNameLabelName)
	}
	if pipelineRun.Labels[ComponentNameLabelName] != "my-component" {
		t.Errorf("generatePaCPipelineRunForComponent(): wrong %s label value", ComponentNameLabelName)
	}
	if pipelineRun.Labels["pipelines.appstudio.openshift.io/type"] != "build" {
		t.Error("generatePaCPipelineRunForComponent(): wrong pipelines.appstudio.openshift.io/type label value")
	}

	if pipelineRun.Annotations["pipelinesascode.tekton.dev/on-cel-expression"] != `event == "pull_request" && target_branch == "custom-branch"` {
		t.Errorf("generatePaCPipelineRunForComponent(): wrong pipelinesascode.tekton.dev/on-cel-expression annotation value")
	}
	if pipelineRun.Annotations["pipelinesascode.tekton.dev/max-keep-runs"] != "3" {
		t.Error("generatePaCPipelineRunForComponent(): wrong pipelinesascode.tekton.dev/max-keep-runs annotation value")
	}
	if pipelineRun.Annotations["build.appstudio.redhat.com/target_branch"] != "{{target_branch}}" {
		t.Error("generatePaCPipelineRunForComponent(): wrong build.appstudio.redhat.com/target_branch annotation value")
	}
	if pipelineRun.Annotations[gitCommitShaAnnotationName] != "{{revision}}" {
		t.Errorf("generatePaCPipelineRunForComponent(): wrong %s annotation value", gitCommitShaAnnotationName)
	}
	if pipelineRun.Annotations[gitRepoAtShaAnnotationName] != "https://githost.com/user/repo?rev={{revision}}" {
		t.Errorf("generatePaCPipelineRunForComponent(): wrong %s annotation value", gitRepoAtShaAnnotationName)
	}
	if pipelineRun.Annotations["build.appstudio.redhat.com/pull_request_number"] != "{{pull_request_number}}" {
		t.Errorf("generatePaCPipelineRunForComponent(): wrong build.appstudio.redhat.com/pull_request_number annotation value")
	}

	if len(pipelineRun.Spec.Params) != 7 {
		t.Error("generatePaCPipelineRunForComponent(): wrong number of pipeline params")
	}
	for _, param := range pipelineRun.Spec.Params {
		switch param.Name {
		case "git-url":
			if param.Value.StringVal != "{{source_url}}" {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s", param.Name)
			}
		case "revision":
			if param.Value.StringVal != "2378a064bf6b66a8ffc650ad88d404cca24ade29" {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "output-image":
			if !strings.HasPrefix(param.Value.StringVal, "registry.io/username/image:on-pr-{{revision}}") {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "rebuild":
			if param.Value.StringVal != "true" {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "image-expires-after":
			if param.Value.StringVal != "5d" {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "dockerfile":
			if param.Value.StringVal != dockerfileURI {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		case "path-context":
			if param.Value.StringVal != dockerfileContext {
				t.Errorf("generatePaCPipelineRunForComponent(): wrong pipeline parameter %s value", param.Name)
			}
		default:
			t.Errorf("generatePaCPipelineRunForComponent(): unexpected pipeline parameter %v", param)
		}
	}

	if len(pipelineRun.Spec.Workspaces) != 2 {
		t.Error("generatePaCPipelineRunForComponent(): wrong number of pipeline workspaces")
	}
	for _, workspace := range pipelineRun.Spec.Workspaces {
		if workspace.Name == "workspace" {
			continue
		}
		if workspace.Name == "git-auth" {
			continue
		}
		t.Errorf("generatePaCPipelineRunForComponent(): unexpected pipeline workspaces %v", workspace)
	}
}

func TestGeneratePaCPipelineRunForComponent_ShouldStopOnDevfileError(t *testing.T) {
	component := &appstudiov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-component",
			Namespace: "my-namespace",
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			Application:    "my-application",
			ContainerImage: "registry.io/username/image:tag",
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL: "https://githost.com/user/repo.git",
					},
				},
			},
		},
		Status: appstudiov1alpha1.ComponentStatus{
			Devfile: getMinimalDevfile(),
		},
	}
	DevfileSearchForDockerfile = func(devfileBytes []byte) (*v1alpha2.DockerfileImage, error) {
		return nil, fmt.Errorf("failed to parse devfile")
	}
	ResetTestGitProviderClient()

	_, err := generatePaCPipelineRunForComponent(component, nil, nil, true, "main", testGitProviderClient)
	DevfileSearchForDockerfile = devfile.SearchForDockerfile
	if err == nil {
		t.Errorf("generatePaCPipelineRunForComponent(): expected error")
	}
	if boErr, ok := err.(*boerrors.BuildOpError); !(ok && boErr.IsPersistent()) {
		t.Errorf("generatePaCPipelineRunForComponent(): expected persistent error")
		if boErr.GetErrorId() != int(boerrors.EInvalidDevfile) {
			t.Errorf("generatePaCPipelineRunForComponent(): expected EInvalidDevfile error")
		}
	}
}

func TestGeneratePaCPipelineRunForComponent_ShouldStopIfTargetBranchIsNotSet(t *testing.T) {
	_, err := generatePaCPipelineRunForComponent(nil, nil, nil, true, "", nil)
	if err == nil {
		t.Errorf("generatePaCPipelineRunForComponent(): expected error")
	}
}

func TestGenerateCelExpressionForPipeline(t *testing.T) {
	componentKey := types.NamespacedName{Namespace: "test-ns", Name: "component-name"}
	ResetTestGitProviderClient()

	tests := []struct {
		name              string
		component         *appstudiov1alpha1.Component
		targetBranch      string
		isDockerfileExist func(repoUrl, branch, dockerfilePath string) (bool, error)
		wantOnPullError   bool
		wantOnPull        string
		wantOnPush        string
	}{
		{
			name: "should generate cel expression for component that occupies whole git repository",
			component: func() *appstudiov1alpha1.Component {
				component := getSampleComponentData(componentKey)
				component.Status.Devfile = getMinimalDevfile()
				return component
			}(),
			targetBranch: "my-branch",
			wantOnPull:   `event == "pull_request" && target_branch == "my-branch"`,
			wantOnPush:   `event == "push" && target_branch == "my-branch"`,
		},
		{
			name: "should generate cel expression for component with context directory, without dokerfile",
			component: func() *appstudiov1alpha1.Component {
				component := getComponentData(componentConfig{componentKey: componentKey, gitSourceContext: "component-dir"})
				component.Status.Devfile = getMinimalDevfile()
				return component
			}(),
			targetBranch: "my-branch",
			wantOnPull:   `event == "pull_request" && target_branch == "my-branch" && ( "component-dir/***".pathChanged() || ".tekton/component-name-pull-request.yaml".pathChanged() )`,
			wantOnPush:   `event == "push" && target_branch == "my-branch"`,
		},
		{
			name: "should generate cel expression for component with context directory and its dockerfile in context directory",
			component: func() *appstudiov1alpha1.Component {
				component := getComponentData(componentConfig{componentKey: componentKey, gitSourceContext: "component-dir"})
				component.Status.Devfile = `
                    schemaVersion: 2.2.0
                    metadata:
                        name: devfile-no-dockerfile
                    components:
                      - name: outerloop-build
                        image:
                            imageName: image:latest
                            dockerfile:
                                uri: docker/Dockerfile
                `
				return component
			}(),
			targetBranch: "my-branch",
			isDockerfileExist: func(repoUrl, branch, dockerfilePath string) (bool, error) {
				return true, nil
			},
			wantOnPull: `event == "pull_request" && target_branch == "my-branch" && ( "component-dir/***".pathChanged() || ".tekton/component-name-pull-request.yaml".pathChanged() )`,
			wantOnPush: `event == "push" && target_branch == "my-branch"`,
		},
		{
			name: "should generate cel expression for component with context directory and its dockerfile outside context directory",
			component: func() *appstudiov1alpha1.Component {
				component := getComponentData(componentConfig{componentKey: componentKey, gitSourceContext: "component-dir"})
				component.Status.Devfile = `
                    schemaVersion: 2.2.0
                    metadata:
                        name: devfile-no-dockerfile
                    components:
                      - name: outerloop-build
                        image:
                            imageName: image:latest
                            dockerfile:
                                uri: docker-root-dir/Dockerfile
                `
				return component
			}(),
			targetBranch: "my-branch",
			isDockerfileExist: func(repoUrl, branch, dockerfilePath string) (bool, error) {
				return false, nil
			},
			wantOnPull: `event == "pull_request" && target_branch == "my-branch" && ( "component-dir/***".pathChanged() || ".tekton/component-name-pull-request.yaml".pathChanged() || "docker-root-dir/Dockerfile".pathChanged() )`,
			wantOnPush: `event == "push" && target_branch == "my-branch"`,
		},
		{
			name: "should generate cel expression for component with context directory and its dockerfile outside git repository",
			component: func() *appstudiov1alpha1.Component {
				component := getComponentData(componentConfig{componentKey: componentKey, gitSourceContext: "component-dir"})
				component.Status.Devfile = `
                    schemaVersion: 2.2.0
                    metadata:
                        name: devfile-no-dockerfile
                    components:
                      - name: outerloop-build
                        image:
                            imageName: image:latest
                            dockerfile:
                                uri: https://host.com:1234/files/Dockerfile
                `
				return component
			}(),
			targetBranch: "my-branch",
			wantOnPull:   `event == "pull_request" && target_branch == "my-branch" && ( "component-dir/***".pathChanged() || ".tekton/component-name-pull-request.yaml".pathChanged() )`,
			wantOnPush:   `event == "push" && target_branch == "my-branch"`,
		},
		{
			name: "should fail to generate cel expression for component if isFileExist fails",
			component: func() *appstudiov1alpha1.Component {
				component := getComponentData(componentConfig{componentKey: componentKey, gitSourceContext: "component-dir"})
				component.Status.Devfile = `
                    schemaVersion: 2.2.0
                    metadata:
                        name: devfile-no-dockerfile
                    components:
                      - name: outerloop-build
                        image:
                            imageName: image:latest
                            dockerfile:
                                uri: docker-root-dir/Dockerfile
                `
				return component
			}(),
			targetBranch: "my-branch",
			isDockerfileExist: func(repoUrl, branch, dockerfilePath string) (bool, error) {
				return false, fmt.Errorf("Failed to check file existance")
			},
			wantOnPullError: true,
			wantOnPush:      `event == "push" && target_branch == "my-branch"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isDockerfileExist != nil {
				IsFileExistFunc = tt.isDockerfileExist
			} else {
				IsFileExistFunc = func(repoUrl, branchName, filePath string) (bool, error) {
					t.Errorf("IsFileExist should not be invoked")
					return false, nil
				}
			}

			got, err := generateCelExpressionForPipeline(tt.component, testGitProviderClient, tt.targetBranch, true)
			if err != nil {
				if !tt.wantOnPullError {
					t.Errorf("generateCelExpressionForPipeline(on pull): got err: %v", err)
				}
			} else {
				if got != tt.wantOnPull {
					t.Errorf("generateCelExpressionForPipeline(on pull): got '%s', want '%s'", got, tt.wantOnPull)
				}
			}

			got, err = generateCelExpressionForPipeline(tt.component, testGitProviderClient, tt.targetBranch, false)
			if err != nil {
				t.Errorf("generateCelExpressionForPipeline(on push): got err: %v", err)
			}
			if got != tt.wantOnPush {
				t.Errorf("generateCelExpressionForPipeline(on push): got '%s', want '%s'", got, tt.wantOnPush)
			}
		})
	}
	ResetTestGitProviderClient()
}

func TestGetContainerImageRepository(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name:  "should not change image",
			image: "image-name",
			want:  "image-name",
		},
		{
			name:  "should not change /user/image",
			image: "user/image",
			want:  "user/image",
		},
		{
			name:  "should not change repository.io/user/image",
			image: "repository.io/user/image",
			want:  "repository.io/user/image",
		},
		{
			name:  "should delete tag",
			image: "repository.io/user/image:tag",
			want:  "repository.io/user/image",
		},
		{
			name:  "should delete sha",
			image: "repository.io/user/image@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "repository.io/user/image",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getContainerImageRepository(tt.image)
			if got != tt.want {
				t.Errorf("getContainerImageRepository(): got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeAndSortTektonParams(t *testing.T) {
	tests := []struct {
		name       string
		existing   []tektonapi.Param
		additional []tektonapi.Param
		want       []tektonapi.Param
	}{
		{
			name: "should merge two different parameters lists",
			existing: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
			},
			additional: []tektonapi.Param{
				{Name: "dockerfile", Value: tektonapi.ParamValue{Type: "string", StringVal: "docker/Dockerfile"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
			},
			want: []tektonapi.Param{
				{Name: "dockerfile", Value: tektonapi.ParamValue{Type: "string", StringVal: "docker/Dockerfile"}},
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
			},
		},
		{
			name: "should append empty parameters list",
			existing: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
			},
			additional: []tektonapi.Param{},
			want: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
			},
		},
		{
			name: "should sort parameters list",
			existing: []tektonapi.Param{
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
				{Name: "dockerfile", Value: tektonapi.ParamValue{Type: "string", StringVal: "docker/Dockerfile"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
			},
			additional: []tektonapi.Param{},
			want: []tektonapi.Param{
				{Name: "dockerfile", Value: tektonapi.ParamValue{Type: "string", StringVal: "docker/Dockerfile"}},
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
			},
		},
		{
			name: "should override existing parameters",
			existing: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "false"}},
			},
			additional: []tektonapi.Param{
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "2378a064bf6b66a8ffc650ad88d404cca24ade29"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
			},
			want: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "2378a064bf6b66a8ffc650ad88d404cca24ade29"}},
			},
		},
		{
			name: "should append and override parameters",
			existing: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "main"}},
			},
			additional: []tektonapi.Param{
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "2378a064bf6b66a8ffc650ad88d404cca24ade29"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
			},
			want: []tektonapi.Param{
				{Name: "git-url", Value: tektonapi.ParamValue{Type: "string", StringVal: "https://githost.com/user/repo.git"}},
				{Name: "rebuild", Value: tektonapi.ParamValue{Type: "string", StringVal: "true"}},
				{Name: "revision", Value: tektonapi.ParamValue{Type: "string", StringVal: "2378a064bf6b66a8ffc650ad88d404cca24ade29"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeAndSortTektonParams(tt.existing, tt.additional)
			if len(got) != len(tt.want) {
				t.Errorf("mergeAndSortTektonParams(): got %v, want %v", got, tt.want)
			}
			for i := range got {
				if !reflect.DeepEqual(got[i], tt.want[i]) {
					t.Errorf("mergeAndSortTektonParams(): got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestGetGitProvider(t *testing.T) {
	type args struct {
		ctx    context.Context
		gitURL string
	}
	tests := []struct {
		name       string
		args       args
		wantErr    bool
		wantString string
	}{
		{
			name: "github ssh",
			args: args{
				ctx:    context.Background(),
				gitURL: "git@github.com:redhat-appstudio/application-service.git",
			},
			wantErr:    false,
			wantString: "https://github.com",
		},
		{
			name: "gitlab ssh",
			args: args{
				ctx:    context.Background(),
				gitURL: "git@gitlab.com:namespace/project-name.git",
			},
			wantErr:    false,
			wantString: "https://gitlab.com",
		},
		{
			name: "bitbucket ssh",
			args: args{
				ctx:    context.Background(),
				gitURL: "git@bitbucket.org:organization/project-name.git",
			},
			wantErr:    false,
			wantString: "https://bitbucket.org",
		},
		{
			name: "github https",
			args: args{
				ctx:    context.Background(),
				gitURL: "https://github.com/redhat-appstudio/application-service.git",
			},
			wantErr:    false,
			wantString: "https://github.com",
		},
		{
			name: "bitbucket https",
			args: args{
				ctx:    context.Background(),
				gitURL: "https://sbose78@bitbucket.org/sbose78/appstudio.git",
			},
			wantErr:    false,
			wantString: "https://bitbucket.org",
		},
		{
			name: "no scheme",
			args: args{
				ctx:    context.Background(),
				gitURL: "github.com/redhat-appstudio/application-service.git",
			},
			wantErr:    true, //fully qualified URL is a must
			wantString: "",
		},
		{
			name: "invalid url",
			args: args{
				ctx:    context.Background(),
				gitURL: "not-even-a-url",
			},
			wantErr:    true, //fully qualified URL is a must
			wantString: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := getGitProviderUrl(tt.args.gitURL); (got != tt.wantString) ||
				(tt.wantErr == true && err == nil) ||
				(tt.wantErr == false && err != nil) {
				t.Errorf("UpdateServiceAccountIfSecretNotLinked() Got Error: = %v, want %v ; Got String:  = %v , want %v", err, tt.wantErr, got, tt.wantString)
			}
		})
	}
}

func TestValidatePaCConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		gitProvider string
		config      map[string][]byte
		expectError bool
	}{
		{
			name:        "should accept GitHub application configuration",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
			},
			expectError: false,
		},
		{
			name:        "should accept GitHub application configuration with end line",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub + "\n"),
			},
			expectError: false,
		},
		{
			name:        "should accept GitHub webhook configuration",
			gitProvider: "github",
			config: map[string][]byte{
				"github.token": []byte("ghp_token"),
			},
			expectError: false,
		},
		{
			name:        "should reject empty GitHub webhook token",
			gitProvider: "github",
			config: map[string][]byte{
				"github.token": []byte(""),
			},
			expectError: true,
		},
		{
			name:        "should accept GitHub application configuration if both GitHub application and webhook configured",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
				"github.token":                          []byte("ghp_token"),
				"gitlab.token":                          []byte("token"),
			},
			expectError: false,
		},
		{
			name:        "should reject GitHub application configuration if GitHub id is missing",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
				"github.token":                          []byte("ghp_token"),
			},
			expectError: true,
		},
		{
			name:        "should reject GitHub application configuration if GitHub application private key is missing",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey: []byte("12345"),
				"github.token":                        []byte("ghp_token"),
			},
			expectError: true,
		},
		{
			name:        "should reject GitHub application configuration if GitHub application id is invalid",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12ab"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
			},
			expectError: true,
		},
		{
			name:        "should reject GitHub application configuration if GitHub application application private key is invalid",
			gitProvider: "github",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte("private-key"),
			},
			expectError: true,
		},
		{
			name:        "should accept GitLab webhook configuration",
			gitProvider: "gitlab",
			config: map[string][]byte{
				"gitlab.token": []byte("token"),
			},
			expectError: false,
		},
		{
			name:        "should accept GitLab webhook configuration even if other providers configured",
			gitProvider: "gitlab",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
				"github.token":                          []byte("ghp_token"),
				"gitlab.token":                          []byte("token"),
				"bitbucket.token":                       []byte("token2"),
			},
			expectError: false,
		},
		{
			name:        "should reject empty GitLab webhook token",
			gitProvider: "gitlab",
			config: map[string][]byte{
				"gitlab.token": []byte(""),
			},
			expectError: true,
		},
		{
			name:        "should accept Bitbucket webhook configuration even if other providers configured",
			gitProvider: "bitbucket",
			config: map[string][]byte{
				gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
				gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
				"github.token":                          []byte("ghp_token"),
				"gitlab.token":                          []byte("token"),
				"bitbucket.token":                       []byte("token2"),
				"username":                              []byte("user"),
			},
			expectError: false,
		},
		{
			name:        "should reject empty Bitbucket webhook token",
			gitProvider: "bitbucket",
			config: map[string][]byte{
				"bitbucket.token": []byte(""),
			},
			expectError: true,
		},
		{
			name:        "test should reject empty Bitbucket username",
			gitProvider: "bitbucket",
			config: map[string][]byte{
				"bitbucket.token": []byte("token"),
				"username":        []byte(""),
			},
			expectError: true,
		},
		{
			name:        "should reject empty Bitbucket webhook token",
			gitProvider: "gitlab",
			config: map[string][]byte{
				"bitbucket.token": []byte(""),
				"username":        []byte("user"),
			},
			expectError: true,
		},
		{
			name:        "should reject Bitbucket webhook configuration with empty username",
			gitProvider: "gitlab",
			config: map[string][]byte{
				"bitbucket.token": []byte("token"),
			},
			expectError: true,
		},
		{
			name:        "should reject unknown application configuration",
			gitProvider: "unknown",
			config:      map[string][]byte{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePaCConfiguration(tt.gitProvider, tt.config)
			if err != nil {
				if !tt.expectError {
					t.Errorf("Expected that the configuration %#v from provider %s should be valid", tt.config, tt.gitProvider)
				}
			} else {
				if tt.expectError {
					t.Errorf("Expected that the configuration %#v from provider %s fails", tt.config, tt.gitProvider)
				}
			}
		})
	}
}

func TestGeneratePaCWebhookSecretString(t *testing.T) {
	expectedSecretStringLength := 20 * 2 // each byte is represented by 2 hex chars

	t.Run("should be able to generate webhook secret string", func(t *testing.T) {
		secret := generatePaCWebhookSecretString()
		if len(secret) != expectedSecretStringLength {
			t.Errorf("Expected that webhook secret string has length %d, but got %d", expectedSecretStringLength, len(secret))
		}
	})

	t.Run("should generate different webhook secret strings", func(t *testing.T) {
		n := 100
		secrets := make([]string, n)
		for i := 0; i < n; i++ {
			secrets[i] = generatePaCWebhookSecretString()
		}

		secret := secrets[0]
		for i := 1; i < n; i++ {
			if secret == secrets[i] {
				t.Errorf("All webhook secrets strings must be different")
			}
		}
	})
}

func TestGetPathContext(t *testing.T) {
	tests := []struct {
		name              string
		gitContext        string
		dockerfileContext string
		want              string
	}{
		{
			name:              "should return empty context if both contexts empty",
			gitContext:        "",
			dockerfileContext: "",
			want:              "",
		},
		{
			name:              "should use current directory from git context",
			gitContext:        ".",
			dockerfileContext: "",
			want:              ".",
		},
		{
			name:              "should use current directory from dockerfile context",
			gitContext:        "",
			dockerfileContext: ".",
			want:              ".",
		},
		{
			name:              "should use current directory if both contexts are current directory",
			gitContext:        ".",
			dockerfileContext: ".",
			want:              ".",
		},
		{
			name:              "should use git context if dockerfile context if not set",
			gitContext:        "dir",
			dockerfileContext: "",
			want:              "dir",
		},
		{
			name:              "should use dockerfile context if git context if not set",
			gitContext:        "",
			dockerfileContext: "dir",
			want:              "dir",
		},
		{
			name:              "should use git context if dockerfile context is current directory",
			gitContext:        "dir",
			dockerfileContext: ".",
			want:              "dir",
		},
		{
			name:              "should use dockerfile context if git context is current directory",
			gitContext:        ".",
			dockerfileContext: "dir",
			want:              "dir",
		},
		{
			name:              "should respect both git and dockerfile contexts",
			gitContext:        "component-dir",
			dockerfileContext: "dockerfile-dir",
			want:              "component-dir/dockerfile-dir",
		},
		{
			name:              "should respect both git and dockerfile contexts in subfolders",
			gitContext:        "path/to/component",
			dockerfileContext: "path/to/dockercontext/",
			want:              "path/to/component/path/to/dockercontext",
		},
		{
			name:              "should remove slash at the end",
			gitContext:        "path/to/dir/",
			dockerfileContext: "",
			want:              "path/to/dir",
		},
		{
			name:              "should remove slash at the end",
			gitContext:        "",
			dockerfileContext: "path/to/dir/",
			want:              "path/to/dir",
		},
		{
			name:              "should not allow absolute path",
			gitContext:        "/path/to/dir/",
			dockerfileContext: "",
			want:              "path/to/dir",
		},
		{
			name:              "should not allow absolute path",
			gitContext:        "",
			dockerfileContext: "/path/to/dir/",
			want:              "path/to/dir",
		},
		{
			name:              "should not allow absolute path",
			gitContext:        "/path/to/dir1/",
			dockerfileContext: "/path/to/dir2/",
			want:              "path/to/dir1/path/to/dir2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPathContext(tt.gitContext, tt.dockerfileContext)
			if got != tt.want {
				t.Errorf("Expected \"%s\", but got \"%s\"", tt.want, got)
			}
		})
	}
}

func TestCreateWorkspaceBinding(t *testing.T) {
	tests := []struct {
		name                      string
		pipelineWorkspaces        []tektonapi.PipelineWorkspaceDeclaration
		expectedWorkspaceBindings []tektonapi.WorkspaceBinding
	}{
		{
			name: "should not bind unknown workspaces",
			pipelineWorkspaces: []tektonapi.PipelineWorkspaceDeclaration{
				{
					Name: "unknown1",
				},
				{
					Name: "unknown2",
				},
			},
			expectedWorkspaceBindings: []tektonapi.WorkspaceBinding{},
		},
		{
			name: "should bind git-auth",
			pipelineWorkspaces: []tektonapi.PipelineWorkspaceDeclaration{
				{
					Name: "git-auth",
				},
			},
			expectedWorkspaceBindings: []tektonapi.WorkspaceBinding{
				{
					Name:   "git-auth",
					Secret: &corev1.SecretVolumeSource{SecretName: "{{ git_auth_secret }}"},
				},
			},
		},
		{
			name: "should bind git-auth and workspace, should not bind unknown",
			pipelineWorkspaces: []tektonapi.PipelineWorkspaceDeclaration{
				{
					Name: "git-auth",
				},
				{
					Name: "unknown",
				},
				{
					Name: "workspace",
				},
			},
			expectedWorkspaceBindings: []tektonapi.WorkspaceBinding{
				{
					Name:   "git-auth",
					Secret: &corev1.SecretVolumeSource{SecretName: "{{ git_auth_secret }}"},
				},
				{
					Name:                "workspace",
					VolumeClaimTemplate: generateVolumeClaimTemplate(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createWorkspaceBinding(tt.pipelineWorkspaces)
			if !reflect.DeepEqual(got, tt.expectedWorkspaceBindings) {
				t.Errorf("Expected %#v, but received %#v", tt.expectedWorkspaceBindings, got)
			}
		})
	}
}

func TestGetRandomString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "should be able to generate one symbol rangom string",
			length: 1,
		},
		{
			name:   "should be able to generate rangom string",
			length: 5,
		},
		{
			name:   "should be able to generate long rangom string",
			length: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRandomString(tt.length)
			if len(got) != tt.length {
				t.Errorf("Got string %s has lenght %d but expected length is %d", got, len(got), tt.length)
			}
		})
	}
}

/*
add new
override existing one
override existing one and unset
*/

func TestPaCRepoAddParamWorkspace(t *testing.T) {
	const workspaceName = "someone-tenant"

	pacConfig := map[string][]byte{
		gitops.PipelinesAsCode_githubAppIdKey:   []byte("12345"),
		gitops.PipelinesAsCode_githubPrivateKey: []byte(ghAppPrivateKeyStub),
	}

	component := getComponentData(componentConfig{})

	convertCustomParamsToMap := func(repository *pacv1alpha1.Repository) map[string]pacv1alpha1.Params {
		result := map[string]pacv1alpha1.Params{}
		for _, param := range *repository.Spec.Params {
			result[param.Name] = param
		}
		return result
	}

	t.Run("add to Spec.Params", func(t *testing.T) {
		repository, _ := gitops.GeneratePACRepository(*component, pacConfig)
		pacRepoAddParamWorkspaceName(log, repository, workspaceName)

		params := convertCustomParamsToMap(repository)
		param, ok := params[pacCustomParamAppstudioWorkspace]
		assert.Assert(t, ok)
		assert.Equal(t, workspaceName, param.Value)
	})

	t.Run("override existing workspace parameter, unset other fields btw", func(t *testing.T) {
		repository, _ := gitops.GeneratePACRepository(*component, pacConfig)
		params := []pacv1alpha1.Params{
			{
				Name:      pacCustomParamAppstudioWorkspace,
				Value:     "another_workspace",
				Filter:    "pac.event_type == \"pull_request\"",
				SecretRef: &pacv1alpha1.Secret{},
			},
		}
		repository.Spec.Params = &params

		pacRepoAddParamWorkspaceName(log, repository, workspaceName)

		existingParams := convertCustomParamsToMap(repository)
		param, ok := existingParams[pacCustomParamAppstudioWorkspace]
		assert.Assert(t, ok)
		assert.Equal(t, workspaceName, param.Value)

		assert.Equal(t, "", param.Filter)
		assert.Assert(t, param.SecretRef == nil)
	})
}
