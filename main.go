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

package main

import (
	"flag"
	"fmt"

	"os"
	"regexp"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	uberzap "go.uber.org/zap"
	uberzapcore "go.uber.org/zap/zapcore"
	"k8s.io/client-go/discovery"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	releaseapi "github.com/konflux-ci/release-service/api/v1alpha1"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"github.com/konflux-ci/build-service/controllers"
	"github.com/konflux-ci/build-service/pkg/bometrics"
	"github.com/konflux-ci/build-service/pkg/k8s"
	l "github.com/konflux-ci/build-service/pkg/logs"
	"github.com/konflux-ci/build-service/pkg/renovate"
	"github.com/konflux-ci/build-service/pkg/webhook"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog logr.Logger
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(appstudiov1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookConfigPath string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(
		&webhookConfigPath,
		"webhook-config-path",
		"",
		"Path to a file that contains webhook configurations",
	)

	zapOpts := zap.Options{
		TimeEncoder: uberzapcore.ISO8601TimeEncoder,
		ZapOpts:     []uberzap.Option{uberzap.WithCaller(true)},
	}
	zapOpts.BindFlags(flag.CommandLine)
	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	setupLog = ctrl.Log.WithName("setup")
	klog.SetLogger(setupLog)

	if err := routev1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add openshift route api to the scheme")
		os.Exit(1)
	}

	if err := tektonapi.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add tekton api to the scheme")
		os.Exit(1)
	}

	if err := pacv1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add pipelinesascode api to the scheme")
		os.Exit(1)
	}
	if err := releaseapi.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add pipelinesascode api to the scheme")
		os.Exit(1)
	}

	clientOpts := client.Options{
		Cache: &client.CacheOptions{
			DisableFor: getCacheExcludedObjectsTypes(),
		},
	}
	metricsOpts := server.Options{
		BindAddress: metricsAddr,
	}

	options := ctrl.Options{
		Client:                 clientOpts,
		Scheme:                 scheme,
		Cache:                  getCacheOptions(),
		Metrics:                metricsOpts,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5483be8f.redhat.com",
	}
	restConfig := ctrl.GetConfigOrDie()

	ensureRequiredAPIGroupsAndResourcesExist(restConfig)

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	webhookConfig, err := webhook.LoadMappingFromFile(webhookConfigPath, os.ReadFile)
	if err != nil {
		setupLog.Error(err, "Failed to load webhook config file", "path", webhookConfigPath)
		os.Exit(1)
	}

	if err = (&controllers.ComponentBuildReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		EventRecorder:      mgr.GetEventRecorderFor("ComponentOnboarding"),
		WebhookURLLoader:   webhook.NewConfigWebhookURLLoader(webhookConfig),
		CredentialProvider: k8s.NewGitCredentialProvider(mgr.GetClient()),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ComponentOnboarding")
		os.Exit(1)
	}

	if err = (&controllers.PaCPipelineRunPrunerReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("PaCPipelineRunPruner"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PaCPipelineRunPruner")
		os.Exit(1)
	}

	if err = (controllers.NewDefaultGitTektonResourcesRenovater(mgr.GetClient(), mgr.GetScheme(), mgr.GetEventRecorderFor("GitTektonResourcesRenovater"))).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitTektonResourcesRenovater")
		os.Exit(1)
	}

	if err = (&controllers.ComponentDependencyUpdateReconciler{
		Client:                       mgr.GetClient(),
		ApiReader:                    mgr.GetAPIReader(),
		EventRecorder:                mgr.GetEventRecorderFor("ComponentDependencyUpdateReconciler"),
		ComponentDependenciesUpdater: renovate.NewDefaultComponentDependenciesUpdater(mgr.GetClient(), mgr.GetScheme(), mgr.GetEventRecorderFor("ComponentDependencyUpdateReconciler")),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PaCPipelineRunPruner")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	if prImageExpiration := os.Getenv(controllers.PipelineRunOnPRExpirationEnvVar); prImageExpiration != "" {
		validExpiration, _ := regexp.Match("^[1-9][0-9]{0,2}[hdw]$", []byte(prImageExpiration))
		if !validExpiration {
			setupLog.Info(fmt.Sprintf("invalid expiration '%s' in %s environment variable, using default %s",
				prImageExpiration, controllers.PipelineRunOnPRExpirationEnvVar, controllers.PipelineRunOnPRExpirationDefault), l.Audit, "true")
			if err := os.Setenv(controllers.PipelineRunOnPRExpirationEnvVar, controllers.PipelineRunOnPRExpirationDefault); err != nil {
				setupLog.Error(err, "unable to set default PipelineRun expiration environment variable")
				os.Exit(1)
			}
		}
	}

	ctx := ctrl.SetupSignalHandler()
	buildMetrics := bometrics.NewBuildMetrics([]bometrics.AvailabilityProbe{bometrics.NewGithubAppAvailabilityProbe(mgr.GetClient())})
	if err := buildMetrics.InitMetrics(metrics.Registry); err != nil {
		setupLog.Error(err, "unable to initialize metrics")
		os.Exit(1)
	}
	buildMetrics.StartAvailabilityProbes(ctx)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getCacheExcludedObjectsTypes() []client.Object {
	return []client.Object{
		&corev1.Secret{},
		&corev1.ConfigMap{},
	}
}

func getCacheOptions() cache.Options {
	componentPipelineRunRequirement, err := labels.NewRequirement(controllers.ComponentNameLabelName, selection.Exists, []string{})
	if err != nil {
		// With valid arguments for the requirement above, the error is always nil.
		panic(err)
	}
	appStudioComponentPipelineRunSelector := labels.NewSelector().Add(*componentPipelineRunRequirement)

	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&tektonapi.PipelineRun{}: {
				Label: appStudioComponentPipelineRunSelector,
			},
			&releaseapi.ReleasePlanAdmission{}: {},
		},
	}
}

func ensureRequiredAPIGroupsAndResourcesExist(restConfig *rest.Config) {
	// Do not start the operator until all of the below is available
	requiredGroupsAndResources := map[string][]string{
		"tekton.dev": {
			"pipelineruns",
		},
		"appstudio.redhat.com": {
			"components",
			"applications",
		},
		"pipelinesascode.tekton.dev": {
			"repositories",
		},
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "failed to create discovery client")
		os.Exit(1)
	}

	delay := 5 * time.Second
	attempts := 60
	for i := 0; i < attempts; i++ {
		if isRequiredAPIGroupsAndResourcesExist(requiredGroupsAndResources, discoveryClient) {
			return
		}
		time.Sleep(delay)
	}

	setupLog.Error(err, "timed out waiting for required API Groups and Resources")
	os.Exit(1)
}

func isRequiredAPIGroupsAndResourcesExist(requiredGroupsAndResources map[string][]string, discoveryClient *discovery.DiscoveryClient) bool {
	apiGroups, apiResources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		setupLog.Error(err, "failed to get ServerGroups using discovery client")
		return false
	}

NextGroup:
	for requiredGroupName, requiredGroupResources := range requiredGroupsAndResources {
		// Search for the given group in all groups list
		for _, group := range apiGroups {
			if group.Name == requiredGroupName {
				// Required group exists
				// Check for required resources of the group
			NextResource:
				for _, requiredGroupResource := range requiredGroupResources {
					for _, apiResource := range apiResources {
						groupName := strings.Split(apiResource.GroupVersion, "/")[0]
						if groupName == requiredGroupName {
							for _, apiResourceInGroup := range apiResource.APIResources {
								if apiResourceInGroup.Name == requiredGroupResource {
									continue NextResource
								}
							}
							// The resource is not found in this version of the group
						}
					}
					// Required API resource is not found in the list of available API resources
					setupLog.Info(fmt.Sprintf("Waiting for %s API Resourse under %s API Group", requiredGroupResource, requiredGroupName))
					return false
				}
				// All API resources from the group is available
				continue NextGroup
			}
		}
		// Required group is not found in the list of available groups
		setupLog.Info(fmt.Sprintf("Waiting for %s API Group", requiredGroupName))
		return false
	}

	return true
}
