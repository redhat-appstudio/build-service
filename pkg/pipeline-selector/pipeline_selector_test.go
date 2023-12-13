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

package pipelineselector

import (
	"reflect"
	"testing"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	buildappstudiov1alpha1 "github.com/redhat-appstudio/build-service/api/v1alpha1"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getBoolPtr(arg bool) *bool {
	return &arg
}

// Return a pipelineRef that refers to a pipeline with the specified name in the specified bundle
func newBundleResolverPipelineRef(bundle, name string) buildappstudiov1alpha1.PipelineRef {
	return buildappstudiov1alpha1.PipelineRef{
		ResolverRef: buildappstudiov1alpha1.ResolverRef{
			Resolver: "bundles",
			Params: []buildappstudiov1alpha1.Param{
				{Name: "kind", Value: *buildappstudiov1alpha1.NewStructuredValues("pipeline")},
				{Name: "bundle", Value: *buildappstudiov1alpha1.NewStructuredValues(bundle)},
				{Name: "name", Value: *buildappstudiov1alpha1.NewStructuredValues(name)},
			},
		},
	}
}

func TestSelectPipelineForComponent(t *testing.T) {
	tests := []struct {
		name               string
		component          *appstudiov1alpha1.Component
		selectors          []buildappstudiov1alpha1.BuildPipelineSelector
		wantPipelineRef    *tektonapi.PipelineRef
		wantPipelineParams []tektonapi.Param
		wantErr            bool
	}{
		{
			name: "Should select pipeline for component",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-component",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"builder":               "maven",
						"additional-checks":     "true",
						"some-other-annotation": "some-annotation-value",
					},
					Labels: map[string]string{
						"builder":           "maven",
						"additional-checks": "true",
						"some-other-label":  "some-label-value",
					},
				},
				Spec: appstudiov1alpha1.ComponentSpec{
					Application:   "test-application",
					ComponentName: "test-component",
				},
				Status: appstudiov1alpha1.ComponentStatus{
					Devfile: `
                        schemaVersion: 2.2.0
                        metadata:
                            name: devfile-no-dockerfile
                            language: java
                            projectType: quarkus
                            starterProjects:
                              - name: community
                                zip:
                                    location: https://code.quarkus.io/d?e=io.quarkus%3Aquarkus-resteasy&e=io.quarkus%3Aquarkus-micrometer&e=io.quarkus%3Aquarkus-smallrye-health&e=io.quarkus%3Aquarkus-openshift&cn=devfile
                        components:
                          - name: outerloop-build    
                            image:
                                imageName: java-quarkus-image:latest
                                dockerfile:
                                    buildContext: .
                                    rootRequired: false
                                    uri: src/main/docker/Dockerfile.jvm.staged
                          - name: tools
                            container:
                                dedicatedPod: true
                                endpoints:
                                  - name: 8080-http
                                    secure: false
                                    targetPort: 8080
                                image: quay.io/eclipse/che-quarkus:7.36.0
                                memoryLimit: 1512Mi
                                mountSources: true
                                volumeMounts:
                                  - name: m2
                                    path: /home/user/.m2
                          - name: m2
                            volume:
                                ephemeral: false
                                size: 2Gi
                        commands:
                          - id: compile
                            exec:
                                commandLine: mvn compile
                                component: tools
                                hotReloadCapable: false
                                workingDir: $PROJECTS_ROOT
                    `,
				},
			},
			selectors: []buildappstudiov1alpha1.BuildPipelineSelector{
				{
					Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
						Selectors: []buildappstudiov1alpha1.PipelineSelector{
							{
								Name: "Java spring",
								PipelineRef: newBundleResolverPipelineRef(
									"java-spring-bundle",
									"java-spring-build-pipeline",
								),
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language:    "java",
									ProjectType: "spring,springboot",
								},
							},
							{
								Name: "Java quarkus for specific components",
								PipelineRef: newBundleResolverPipelineRef(
									"quarkus-for-specific-components-bundle",
									"quarkus-for-specific-components-build-pipeline",
								),
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language:      "java",
									ProjectType:   "quarkus",
									ComponentName: "quarkus,test-quarkus",
								},
							},
						},
					},
				},
				{
					Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
						Selectors: []buildappstudiov1alpha1.PipelineSelector{
							{
								Name: "Java quarkus no dockerfile",
								PipelineRef: newBundleResolverPipelineRef(
									"quarkus-no-dockerfile-bundle",
									"quarkus-no-dockerfile-build-pipeline",
								),
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language:           "java",
									ProjectType:        "quarkus",
									DockerfileRequired: getBoolPtr(false),
								},
							},
							{
								Name: "Java quarkus label restricted",
								PipelineRef: newBundleResolverPipelineRef(
									"quarkus-label-restricted-bundle",
									"quarkus-label-restricted-build-pipeline",
								),
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language:    "java",
									ProjectType: "quarkus",
									Labels: map[string]string{
										"allowAllComponents": "No",
									},
								},
							},
						},
					},
				},
				{
					Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
						Selectors: []buildappstudiov1alpha1.PipelineSelector{
							{
								Name: "Java gradle",
								PipelineRef: newBundleResolverPipelineRef(
									"quarkus-gradle-bundle",
									"quarkus-gradle-build-pipeline",
								),
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language:    "java",
									ProjectType: "quarkus",
									Labels: map[string]string{
										"builder": "gradle",
									},
								},
							},
							{
								Name: "Java quarkus for the component in this test",
								PipelineRef: newBundleResolverPipelineRef(
									"quarkus-bundle",
									"quarkus-build-pipeline",
								),
								PipelineParams: []buildappstudiov1alpha1.PipelineParam{
									{
										Name:  "pipeline-param",
										Value: "quarkus-test-param",
									},
								},
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language:    "java",
									ProjectType: "quarkus",
								},
							},
							{
								Name: "All other java",
								PipelineRef: newBundleResolverPipelineRef(
									"java-bundle",
									"java-build-pipeline",
								),
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language: "java",
								},
							},
						},
					},
				},
				{
					Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
						Selectors: []buildappstudiov1alpha1.PipelineSelector{
							{
								Name: "Fallbackg",
								PipelineRef: newBundleResolverPipelineRef(
									"my-bundle",
									"default-build-pipeline",
								),
							},
						},
					},
				},
			},
			wantPipelineRef: &tektonapi.PipelineRef{ResolverRef: tektonapi.ResolverRef{
				Resolver: "bundles",
				Params: []tektonapi.Param{
					{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
					{Name: "bundle", Value: *tektonapi.NewStructuredValues("quarkus-bundle")},
					{Name: "name", Value: *tektonapi.NewStructuredValues("quarkus-build-pipeline")},
				},
			}},
			wantPipelineParams: []tektonapi.Param{
				{
					Name:  "pipeline-param",
					Value: *tektonapi.NewStructuredValues("quarkus-test-param"),
				},
			},
			wantErr: false,
		},
		{
			name: "Should return nil if nothing matches",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-component",
					Namespace: "test-namespace",
				},
				Status: appstudiov1alpha1.ComponentStatus{
					Devfile: `
                        schemaVersion: 2.2.0
                        metadata:
                            name: test-devfile
                            language: go
                    `,
				},
			},
			selectors: []buildappstudiov1alpha1.BuildPipelineSelector{
				{
					Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
						Selectors: []buildappstudiov1alpha1.PipelineSelector{
							{
								Name: "Python",
								PipelineRef: newBundleResolverPipelineRef(
									"my-bundle",
									"python-build-pipeline",
								),
								PipelineParams: []buildappstudiov1alpha1.PipelineParam{
									{
										Name:  "arg1",
										Value: "val1",
									},
								},
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language: "python",
								},
							},
							{
								Name: "NodeJS",
								PipelineRef: newBundleResolverPipelineRef(
									"my-bundle",
									"nodejs-build-pipeline",
								),
								PipelineParams: []buildappstudiov1alpha1.PipelineParam{
									{
										Name:  "arg2",
										Value: "val2",
									},
								},
								WhenConditions: buildappstudiov1alpha1.WhenCondition{
									Language: "nodejs",
								},
							},
						},
					},
				},
			},
			wantPipelineRef:    nil,
			wantPipelineParams: nil,
			wantErr:            false,
		},
		{
			name: "Should return nil if no selectors given",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-component",
					Namespace: "test-namespace",
				},
				Status: appstudiov1alpha1.ComponentStatus{
					Devfile: `
                        schemaVersion: 2.2.0
                        metadata:
                            name: test-devfile
                    `,
				},
			},
			selectors:          nil,
			wantPipelineRef:    nil,
			wantPipelineParams: nil,
			wantErr:            false,
		},
		{
			name: "Should fail if component devfile is invalid",
			component: &appstudiov1alpha1.Component{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-component",
					Namespace: "test-namespace",
				},
				Status: appstudiov1alpha1.ComponentStatus{
					Devfile: `wrongFiled: value`,
				},
			},
			selectors: []buildappstudiov1alpha1.BuildPipelineSelector{
				{
					Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
						Selectors: []buildappstudiov1alpha1.PipelineSelector{
							{
								Name: "Fallback",
								PipelineRef: newBundleResolverPipelineRef(
									"my-bundle",
									"default-build-pipeline",
								),
							},
						},
					},
				},
			},
			wantPipelineRef:    nil,
			wantPipelineParams: nil,
			wantErr:            true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelineRef, pipelineParams, err := SelectPipelineForComponent(tt.component, tt.selectors)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SelectPipelineForComponent(): expected error on component: %v\n", tt.component)
				}
				return
			}
			if err != nil {
				t.Errorf("SelectPipelineForComponent(): unexpected error: %s on component: %v\n", err.Error(), tt.component)
				return
			}
			if !reflect.DeepEqual(pipelineRef, tt.wantPipelineRef) {
				t.Errorf("SelectPipelineForComponent(): pipelineRef got: %v, want: %v", pipelineRef, tt.wantPipelineRef)
			}
			if !reflect.DeepEqual(pipelineParams, tt.wantPipelineParams) {
				t.Errorf("SelectPipelineForComponent(): pipelineParams got: %v, want: %v", pipelineParams, tt.wantPipelineParams)
			}
		})
	}
}

func TestGetPipelineSelectionParametersForComponent(t *testing.T) {
	getComponent := func(devfileYaml string) appstudiov1alpha1.Component {
		return appstudiov1alpha1.Component{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-component",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					"builder":               "maven",
					"additional-checks":     "true",
					"some-other-annotation": "some-annotation-value",
				},
				Labels: map[string]string{
					"builder":           "maven",
					"additional-checks": "true",
					"some-other-label":  "some-label-value",
				},
			},
			Spec: appstudiov1alpha1.ComponentSpec{
				Application:   "test-application",
				ComponentName: "test-component",
			},
			Status: appstudiov1alpha1.ComponentStatus{
				Devfile: devfileYaml,
			},
		}
	}
	getPipelineSelectionConditions := func() buildappstudiov1alpha1.WhenCondition {
		return buildappstudiov1alpha1.WhenCondition{
			DockerfileRequired: getBoolPtr(false),
			ComponentName:      "test-component",
			Annotations: map[string]string{
				"builder":               "maven",
				"additional-checks":     "true",
				"some-other-annotation": "some-annotation-value",
			},
			Labels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
				"some-other-label":  "some-label-value",
			},
		}
	}

	tests := []struct {
		name           string
		component      appstudiov1alpha1.Component
		wantConditions buildappstudiov1alpha1.WhenCondition
		wantErr        bool
	}{
		{
			name: "should get component parametes with minimal devfile",
			component: getComponent(`        
                schemaVersion: 2.2.0
                metadata:
                    name: minimal-devfile
            `),
			wantConditions: getPipelineSelectionConditions(),
			wantErr:        false,
		},
		{
			name: "should get component parametes without dockerfile",
			component: getComponent(`        
                schemaVersion: 2.2.0
                metadata:
                    name: devfile-no-dockerfile
                    language: java
                    projectType: quarkus
                    starterProjects:
                      - name: community
                        zip:
                            location: https://code.quarkus.io/d?e=io.quarkus%3Aquarkus-resteasy&e=io.quarkus%3Aquarkus-micrometer&e=io.quarkus%3Aquarkus-smallrye-health&e=io.quarkus%3Aquarkus-openshift&cn=devfile
                components:
                  - name: outerloop-deploy
                    kubernetes:
                      inlined: |-
                        kind: Deployment
                        apiVersion: apps/v1
                        metadata:
                          name: quarkus
                          spec:
                            replicas: 1
                            selector:
                              matchLabels:
                                app: quarkus
                            template:
                              metadata:
                                labels:
                                  app: quarkus
                              spec:
                                containers:
                                - name: quarkus
                                  image: hello-world:latest
                                  ports:
                                  - name: http
                                    containerPort: 8080
                                    protocol: TCP
                                  resources:
                                    limits:
                                      memory: "1024Mi"
                                      cpu: "500m"
                  - name: tools
                    container:
                        dedicatedPod: true
                        endpoints:
                          - name: 8080-http
                            secure: false
                            targetPort: 8080
                        image: quay.io/eclipse/che-quarkus:7.36.0
                        memoryLimit: 1512Mi
                        mountSources: true
                        volumeMounts:
                          - name: m2
                            path: /home/user/.m2
                  - name: m2
                    volume:
                        ephemeral: false
                        size: 2Gi
                commands:
                  - id: compile
                    exec:
                        commandLine: mvn compile
                        component: tools
                        hotReloadCapable: false
                        workingDir: $PROJECTS_ROOT
            `),
			wantConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getPipelineSelectionConditions()
				conditions.Language = "java"
				conditions.ProjectType = "quarkus"
				conditions.DockerfileRequired = getBoolPtr(false)
				return conditions
			}(),
			wantErr: false,
		},
		{
			name: "should get component parametes with dockerfile",
			component: getComponent(`        
                schemaVersion: 2.2.0
                metadata:
                    name: devfile-no-dockerfile
                    language: java
                    projectType: quarkus
                    starterProjects:
                      - name: community
                        zip:
                            location: https://code.quarkus.io/d?e=io.quarkus%3Aquarkus-resteasy&e=io.quarkus%3Aquarkus-micrometer&e=io.quarkus%3Aquarkus-smallrye-health&e=io.quarkus%3Aquarkus-openshift&cn=devfile
                components:
                  - name: outerloop-build	
                    image:
                        imageName: java-quarkus-image:latest
                        dockerfile:
                            buildContext: .
                            rootRequired: false
                            uri: src/main/docker/Dockerfile.jvm.staged
                  - name: tools
                    container:
                        dedicatedPod: true
                        endpoints:
                          - name: 8080-http
                            secure: false
                            targetPort: 8080
                        image: quay.io/eclipse/che-quarkus:7.36.0
                        memoryLimit: 1512Mi
                        mountSources: true
                        volumeMounts:
                          - name: m2
                            path: /home/user/.m2
                  - name: m2
                    volume:
                        ephemeral: false
                        size: 2Gi
                commands:
                  - id: compile
                    exec:
                        commandLine: mvn compile
                        component: tools
                        hotReloadCapable: false
                        workingDir: $PROJECTS_ROOT
            `),
			wantConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getPipelineSelectionConditions()
				conditions.Language = "java"
				conditions.ProjectType = "quarkus"
				conditions.DockerfileRequired = getBoolPtr(true)
				return conditions
			}(),
			wantErr: false,
		},
		{
			name: "should return error for invalid devfile",
			component: getComponent(`        
                schemaVersion: 2.2.0
                metadata:
                    name: wrong-devfile
                unexisted:
                    key: value
            `),
			wantConditions: buildappstudiov1alpha1.WhenCondition{},
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelineSelectionConditions, err := getPipelineSelectionParametersForComponent(&tt.component)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getPipelineSelectionParametersForCompoent(): expected error on component: %v\n", tt.component)
				}
				return
			}
			if err != nil {
				t.Errorf("getPipelineSelectionParametersForCompoent(): unexpected error: %s on component: %v\n", err.Error(), tt.component)
				return
			}

			if !reflect.DeepEqual(*pipelineSelectionConditions, tt.wantConditions) {
				t.Errorf("getPipelineSelectionParametersForCompoent(): got: %v, want: %v", pipelineSelectionConditions, tt.wantConditions)
			}
		})
	}
}

func TestFindMatchingPipeline(t *testing.T) {
	tests := []struct {
		name                string
		componentConditions buildappstudiov1alpha1.WhenCondition
		pipelinesChain      buildappstudiov1alpha1.BuildPipelineSelector
		wantPipelineRef     *tektonapi.PipelineRef
		wantPipelineParams  []tektonapi.Param
	}{
		{
			name: "should match the only pipeline in chain if conditions are met",
			componentConditions: buildappstudiov1alpha1.WhenCondition{
				Language:    "java",
				ProjectType: "springboot",
			},
			pipelinesChain: buildappstudiov1alpha1.BuildPipelineSelector{
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:    "java",
								ProjectType: "spring,springboot,quarkus",
							},
						},
					},
				},
			},
			wantPipelineRef: &tektonapi.PipelineRef{ResolverRef: tektonapi.ResolverRef{
				Resolver: "bundles",
				Params: []tektonapi.Param{
					{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
					{Name: "bundle", Value: *tektonapi.NewStructuredValues("my-bundle")},
					{Name: "name", Value: *tektonapi.NewStructuredValues("java-build-pipeline")},
				},
			}},
			wantPipelineParams: nil,
		},
		{
			name: "should match the pipeline in the chain",
			componentConditions: buildappstudiov1alpha1.WhenCondition{
				Language:    "java",
				ProjectType: "springboot",
			},
			pipelinesChain: buildappstudiov1alpha1.BuildPipelineSelector{
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name: "Python",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"python-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language: "python",
							},
						},
						{
							Name: "Java",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:    "java",
								ProjectType: "spring,springboot,quarkus",
							},
						},
						{
							Name: "Fallback",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"default-build-pipeline",
							),
						},
					},
				},
			},
			wantPipelineRef: &tektonapi.PipelineRef{ResolverRef: tektonapi.ResolverRef{
				Resolver: "bundles",
				Params: []tektonapi.Param{
					{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
					{Name: "bundle", Value: *tektonapi.NewStructuredValues("my-bundle")},
					{Name: "name", Value: *tektonapi.NewStructuredValues("java-build-pipeline")},
				},
			}},
			wantPipelineParams: nil,
		},
		{
			name: "should match the pipeline in the chain with advanced options",
			componentConditions: buildappstudiov1alpha1.WhenCondition{
				Language:           "java",
				ProjectType:        "springboot",
				ComponentName:      "my-project",
				DockerfileRequired: getBoolPtr(false),
				Labels: map[string]string{
					"builder":           "maven",
					"additional-checks": "true",
					"unrelated-label":   "unrelated-value",
				},
				Annotations: map[string]string{
					"spring-projects":      "true",
					"unrelated-annotation": "unrelated-annotation",
				},
			},
			pipelinesChain: buildappstudiov1alpha1.BuildPipelineSelector{
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							Name: "Should not match another language",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"python-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "python",
								ProjectType:        "springboot",
								ComponentName:      "my-project",
								DockerfileRequired: getBoolPtr(false),
								Labels: map[string]string{
									"builder":           "maven",
									"additional-checks": "true",
								},
								Annotations: map[string]string{
									"spring-projects": "true",
								},
							},
						},
						{
							Name: "Should not match builder label",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"builder-label-java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "java",
								ProjectType:        "spring,springboot,quarkus",
								ComponentName:      "my-project",
								DockerfileRequired: getBoolPtr(false),
								Labels: map[string]string{
									"builder":           "gradle",
									"additional-checks": "true",
								},
								Annotations: map[string]string{
									"spring-projects": "true",
								},
							},
						},
						{
							Name: "Should not match extra label",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"extra-label-java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "java",
								ProjectType:        "spring,springboot,quarkus",
								ComponentName:      "my-project",
								DockerfileRequired: getBoolPtr(false),
								Labels: map[string]string{
									"builder":           "maven",
									"additional-checks": "true",
									"network-type":      "isolated",
								},
								Annotations: map[string]string{
									"spring-projects": "true",
								},
							},
						},
						{
							Name: "Should not match annotation",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"annotation-java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "java",
								ProjectType:        "spring,springboot,quarkus",
								ComponentName:      "my-project",
								DockerfileRequired: getBoolPtr(false),
								Labels: map[string]string{
									"builder":           "maven",
									"additional-checks": "true",
								},
								Annotations: map[string]string{
									"spring-projects": "false",
								},
							},
						},
						{
							Name: "Should not match component name",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"component-name-java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "java",
								ProjectType:        "spring,springboot,quarkus",
								ComponentName:      "not-my-project",
								DockerfileRequired: getBoolPtr(false),
								Labels: map[string]string{
									"builder":           "maven",
									"additional-checks": "true",
								},
								Annotations: map[string]string{
									"spring-projects": "true",
								},
							},
						},
						{
							Name: "Should not match dockerfile presence",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"dockerfile-java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "java",
								ProjectType:        "spring,springboot,quarkus",
								ComponentName:      "my-project",
								DockerfileRequired: getBoolPtr(true),
								Labels: map[string]string{
									"builder":           "maven",
									"additional-checks": "true",
								},
								Annotations: map[string]string{
									"spring-projects": "true",
								},
							},
						},
						{
							Name: "Should match the pipeline",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"right-java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:           "java",
								ProjectType:        "spring,springboot,quarkus",
								ComponentName:      "my-project",
								DockerfileRequired: getBoolPtr(false),
								Labels: map[string]string{
									"builder":           "maven",
									"additional-checks": "true",
								},
								Annotations: map[string]string{
									"spring-projects": "true",
								},
							},
						},
						{
							Name: "Fallback",
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"default-build-pipeline",
							),
						},
					},
				},
			},
			wantPipelineRef: &tektonapi.PipelineRef{ResolverRef: tektonapi.ResolverRef{
				Resolver: "bundles",
				Params: []tektonapi.Param{
					{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
					{Name: "bundle", Value: *tektonapi.NewStructuredValues("my-bundle")},
					{Name: "name", Value: *tektonapi.NewStructuredValues("right-java-build-pipeline")},
				},
			}},
			wantPipelineParams: nil,
		},
		{
			name: "should return nil if no pipeline matches",
			componentConditions: buildappstudiov1alpha1.WhenCondition{
				Language: "C++",
			},
			pipelinesChain: buildappstudiov1alpha1.BuildPipelineSelector{
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"java-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:    "java",
								ProjectType: "spring,springboot,quarkus",
							},
						},
						{
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"python-build-pipeline",
							),
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language: "python",
							},
						},
					},
				},
			},
			wantPipelineRef:    nil,
			wantPipelineParams: nil,
		},
		{
			name: "should return build pipeline params",
			componentConditions: buildappstudiov1alpha1.WhenCondition{
				Language:    "java",
				ProjectType: "springboot",
			},
			pipelinesChain: buildappstudiov1alpha1.BuildPipelineSelector{
				Spec: buildappstudiov1alpha1.BuildPipelineSelectorSpec{
					Selectors: []buildappstudiov1alpha1.PipelineSelector{
						{
							PipelineRef: newBundleResolverPipelineRef(
								"my-bundle",
								"java-build-pipeline",
							),
							PipelineParams: []buildappstudiov1alpha1.PipelineParam{
								{
									Name:  "additional-checks",
									Value: "true",
								},
								{
									Name:  "param2",
									Value: "value2",
								},
							},
							WhenConditions: buildappstudiov1alpha1.WhenCondition{
								Language:    "java",
								ProjectType: "spring,springboot,quarkus",
							},
						},
					},
				},
			},
			wantPipelineRef: &tektonapi.PipelineRef{ResolverRef: tektonapi.ResolverRef{
				Resolver: "bundles",
				Params: []tektonapi.Param{
					{Name: "kind", Value: *tektonapi.NewStructuredValues("pipeline")},
					{Name: "bundle", Value: *tektonapi.NewStructuredValues("my-bundle")},
					{Name: "name", Value: *tektonapi.NewStructuredValues("java-build-pipeline")},
				},
			}},
			wantPipelineParams: []tektonapi.Param{
				{
					Name:  "additional-checks",
					Value: *tektonapi.NewStructuredValues("true"),
				},
				{
					Name:  "param2",
					Value: *tektonapi.NewStructuredValues("value2"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelineRef, pipelineParams := findMatchingPipeline(&tt.componentConditions, &tt.pipelinesChain)

			if !reflect.DeepEqual(pipelineRef, tt.wantPipelineRef) {
				t.Errorf("findMatchingPipeline(): pipelineRef got: %v, want: %v", pipelineRef, tt.wantPipelineRef)
			}
			if !reflect.DeepEqual(pipelineParams, tt.wantPipelineParams) {
				t.Errorf("findMatchingPipeline(): pipelineParams got: %v, want: %v", pipelineParams, tt.wantPipelineParams)
			}
		})
	}
}

func TestPipelineConditionsMatchComponentParameters(t *testing.T) {
	getSampleConditions := func() buildappstudiov1alpha1.WhenCondition {
		return buildappstudiov1alpha1.WhenCondition{
			Language:           "java",
			ProjectType:        "spring",
			DockerfileRequired: getBoolPtr(true),
			ComponentName:      "my-component",
			Annotations: map[string]string{
				"builder":               "maven",
				"additional-checks":     "true",
				"some-other-annotation": "some-annotation-value",
			},
			Labels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
				"some-other-label":  "some-label-value",
			},
		}
	}

	tests := []struct {
		name                string
		componentConditions buildappstudiov1alpha1.WhenCondition
		pipelineConditions  buildappstudiov1alpha1.WhenCondition
		wantMatch           bool
	}{
		{
			name:                "should match if all conditions are the same",
			componentConditions: getSampleConditions(),
			pipelineConditions:  getSampleConditions(),
			wantMatch:           true,
		},
		{
			name: "should match if all component parameters are supported by the pipeline",
			componentConditions: buildappstudiov1alpha1.WhenCondition{
				Language:           "java",
				ProjectType:        "spring",
				DockerfileRequired: getBoolPtr(true),
				ComponentName:      "my-component",
				Annotations: map[string]string{
					"builder":               "maven",
					"additional-checks":     "true",
					"some-other-annotation": "some-annotation-value",
				},
				Labels: map[string]string{
					"builder":           "maven",
					"additional-checks": "false",
					"some-other-label":  "some-label-value",
				},
			},
			pipelineConditions: buildappstudiov1alpha1.WhenCondition{
				Language:           "scala,java,kotlin",
				ProjectType:        "quarkus,spring,springboot",
				DockerfileRequired: getBoolPtr(true),
				ComponentName:      "test-component,my-component,another-component-name",
				Annotations: map[string]string{
					"builder":               "gradle,maven",
					"additional-checks":     "true",
					"some-other-annotation": "some-annotation-value,value",
				},
				Labels: map[string]string{
					"builder":           "maven,gradle",
					"additional-checks": "false",
				},
			},
			wantMatch: true,
		},
		{
			name:                "any component should match empty pipeline condition",
			componentConditions: getSampleConditions(),
			pipelineConditions:  buildappstudiov1alpha1.WhenCondition{},
			wantMatch:           true,
		},
		{
			name:                "should allow any language",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.Language = ""
				return conditions
			}(),
			wantMatch: true,
		},
		{
			name:                "should allow any project type",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.ProjectType = ""
				return conditions
			}(),
			wantMatch: true,
		},
		{
			name:                "should ignore dockerfile flag if it's not set",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.DockerfileRequired = nil
				return conditions
			}(),
			wantMatch: true,
		},
		{
			name:                "should allow any component name",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.ComponentName = ""
				return conditions
			}(),
			wantMatch: true,
		},
		{
			name:                "should allow any labels",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.Labels = make(map[string]string)
				return conditions
			}(),
			wantMatch: true,
		},
		{
			name:                "should allow any annotations",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.Annotations = make(map[string]string)
				return conditions
			}(),
			wantMatch: true,
		},
		{
			name:                "should not match if language does not match",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.Language = "other"
				return conditions
			}(),
			wantMatch: false,
		},
		{
			name:                "should not match if project type does not match",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.ProjectType = "other"
				return conditions
			}(),
			wantMatch: false,
		},
		{
			name:                "should not match if dockerfile flag does not match",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.DockerfileRequired = getBoolPtr(false)
				return conditions
			}(),
			wantMatch: false,
		},
		{
			name:                "should not match if component name does not match",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.ComponentName = "other"
				return conditions
			}(),
			wantMatch: false,
		},
		{
			name:                "should not match if labels does not match",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.Labels = map[string]string{
					"other": "other",
				}
				return conditions
			}(),
			wantMatch: false,
		},
		{
			name:                "should not match if annotations does not match",
			componentConditions: getSampleConditions(),
			pipelineConditions: func() buildappstudiov1alpha1.WhenCondition {
				conditions := getSampleConditions()
				conditions.Annotations = map[string]string{
					"other": "other",
				}
				return conditions
			}(),
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := pipelineConditionsMatchComponentParameters(&tt.pipelineConditions, &tt.componentConditions)
			if matches != tt.wantMatch {
				t.Errorf("pipelineConditionsMatchComponentParameters(%v, %v): got: %t, want: %t", tt.pipelineConditions, tt.componentConditions, matches, tt.wantMatch)
			}
		})
	}
}

func TestPipelineMatchesComponentCondition(t *testing.T) {
	tests := []struct {
		name               string
		componentCondition string
		pipelineConditions string
		wantMatch          bool
	}{
		{
			name:               "should match if conditions are the same",
			componentCondition: "orange",
			pipelineConditions: "orange",
			wantMatch:          true,
		},
		{
			name:               "should match if conditions are the same ignoring case",
			componentCondition: "orange",
			pipelineConditions: "Orange",
			wantMatch:          true,
		},
		{
			name:               "should match if conditions are the same ignoring whitespaces",
			componentCondition: " orange",
			pipelineConditions: "orange ",
			wantMatch:          true,
		},
		{
			name:               "should match if component condition is included into pipeline conditions",
			componentCondition: "plum",
			pipelineConditions: "peach,plum,apricot",
			wantMatch:          true,
		},
		{
			name:               "should match if component condition is included into pipeline conditions ignoring case",
			componentCondition: "plum",
			pipelineConditions: "Peach,Plum,Apricot",
			wantMatch:          true,
		},
		{
			name:               "should match if component condition is included into pipeline conditions ignoring whitespaces",
			componentCondition: "plum",
			pipelineConditions: " peach , plum 	, apricot ",
			wantMatch:          true,
		},
		{
			name:               "should match if component condition is included into pipeline conditions despite empty items",
			componentCondition: "plum",
			pipelineConditions: " , peach , , plum , apricot , ",
			wantMatch:          true,
		},
		{
			name:               "should not match if conditions does not match",
			componentCondition: "orange",
			pipelineConditions: "pear",
			wantMatch:          false,
		},
		{
			name:               "should not match if pipeline conditions does not include component condition",
			componentCondition: "orange",
			pipelineConditions: "peach,plum,apricot",
			wantMatch:          false,
		},
		{
			name:               "should not match if pipeline conditions does not include component condition despite empty items",
			componentCondition: "orange",
			pipelineConditions: " , peach , , plum , apricot , ",
			wantMatch:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := pipelineMatchesComponentCondition(tt.pipelineConditions, tt.componentCondition)
			if matches != tt.wantMatch {
				t.Errorf("pipelineMatchesComponentCondition(%s, %s): got: %v, want: %v", tt.pipelineConditions, tt.componentCondition, matches, tt.wantMatch)
			}
		})
	}
}

func TestPipelineMatchesComponentLabels(t *testing.T) {
	tests := []struct {
		name            string
		componentLabels map[string]string
		pipelineLabels  map[string]string
		wantMatch       bool
	}{
		{
			name: "should match if there is no labels requirements",
			componentLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
			},
			pipelineLabels: map[string]string{},
			wantMatch:      true,
		},
		{
			name: "should match empty value labels",
			componentLabels: map[string]string{
				"env-development": "",
			},
			pipelineLabels: map[string]string{
				"env-development": "",
			},
			wantMatch: true,
		},
		{
			name: "should match if the only label is the same",
			componentLabels: map[string]string{
				"builder": "maven",
			},
			pipelineLabels: map[string]string{
				"builder": "maven",
			},
			wantMatch: true,
		},
		{
			name: "should match if component labels match pipeline requirements",
			componentLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
				"some-other-label":  "some-label-value",
			},
			pipelineLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
			},
			wantMatch: true,
		},
		{
			name: "should match if the only label value are supported by pipeline",
			componentLabels: map[string]string{
				"builder": "maven",
			},
			pipelineLabels: map[string]string{
				"builder": "gradle,maven,manual",
			},
			wantMatch: true,
		},
		{
			name: "should match if component labels are supported by pipeline",
			componentLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
				"some-feature":      "value5",
				"some-other-label":  "some-label-value",
			},
			pipelineLabels: map[string]string{
				"builder":           "maven,gradle",
				"additional-checks": "true",
				"some-feature":      "value1,value2,value5,value9",
			},
			wantMatch: true,
		},
		{
			name: "should match if empty value label is an option",
			componentLabels: map[string]string{
				"empty-value": "",
			},
			pipelineLabels: map[string]string{
				"empty-value": "value1,value2,",
			},
			wantMatch: true,
		},
		{
			name: "should match if component labels are supported by pipeline ignoring whitespaces and case",
			componentLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
				"some-feature":      "value5",
				"some-other-label":  "some-label-value",
			},
			pipelineLabels: map[string]string{
				"builder":           " Test, Maven , gradle ",
				"additional-checks": "True",
				"some-feature":      "  Value1 ,  value2, value5,Value9 ",
			},
			wantMatch: true,
		},
		{
			name: "should not match if the only label is different",
			componentLabels: map[string]string{
				"builder": "maven",
			},
			pipelineLabels: map[string]string{
				"builder": "gradle",
			},
			wantMatch: false,
		},
		{
			name: "should not match if component labels does not match pipeline requirements",
			componentLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
			},
			pipelineLabels: map[string]string{
				"builder":                   "maven",
				"additional-checks":         "true",
				"some-other-required-label": "some-required-value",
			},
			wantMatch: false,
		},
		{
			name: "should not match if the only label value are not supported by pipeline",
			componentLabels: map[string]string{
				"builder": "maven",
			},
			pipelineLabels: map[string]string{
				"builder": "gradle,make,manual",
			},
			wantMatch: false,
		},
		{
			name: "should not match if the only one label does not match",
			componentLabels: map[string]string{
				"builder":           "maven",
				"additional-checks": "true",
				"some-feature":      "value5",
				"some-other-label":  "some-label-value",
			},
			pipelineLabels: map[string]string{
				"builder":           "gradle,maven,manual",
				"additional-checks": "true",
				"some-feature":      "value2,value6,value8",
			},
			wantMatch: false,
		},
		{
			name: "should not match if empty value label is not an option",
			componentLabels: map[string]string{
				"empty-value": "",
			},
			pipelineLabels: map[string]string{
				"empty-value": "value1,value2",
			},
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := pipelineMatchesComponentLabels(tt.pipelineLabels, tt.componentLabels)
			if matches != tt.wantMatch {
				t.Errorf("pipelineMatchesComponentCondition(%s, %s): got: %v, want: %v", tt.pipelineLabels, tt.componentLabels, matches, tt.wantMatch)
			}
		})
	}
}
