/*
Copyright 2023.

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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	. "github.com/konflux-ci/build-service/pkg/common"
	"github.com/konflux-ci/build-service/pkg/git"
	l "github.com/konflux-ci/build-service/pkg/logs"
	"github.com/konflux-ci/build-service/pkg/renovate"
	corev1 "k8s.io/api/core/v1"
)

const (
	NextReconcile = 6 * time.Hour
)

// GitTektonResourcesRenovater watches build pipeline ConfigMap object in order to update
// existing .tekton directories.
type GitTektonResourcesRenovater struct {
	client               client.Client
	eventRecorder        record.EventRecorder
	updateCoordinator    *renovate.UpdateCoordinator
	updateTargetProvider renovate.UpdateTargetProvider
}

func NewDefaultGitTektonResourcesRenovater(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder) *GitTektonResourcesRenovater {
	return NewGitTektonResourcesRenovater(client, scheme, eventRecorder, renovate.NewCompositeUpdateTargetProvider(client, scheme, eventRecorder))
}

func NewGitTektonResourcesRenovater(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder, updateTargetProvider renovate.UpdateTargetProvider) *GitTektonResourcesRenovater {
	return &GitTektonResourcesRenovater{
		client:               client,
		updateTargetProvider: updateTargetProvider,
		eventRecorder:        eventRecorder,
		updateCoordinator:    renovate.NewUpdateCoordinator(client, scheme),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitTektonResourcesRenovater) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&corev1.ConfigMap{}, builder.WithPredicates(predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetNamespace() == BuildServiceNamespaceName && e.Object.GetName() == buildPipelineConfigMapResourceName
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetNamespace() == BuildServiceNamespaceName && e.ObjectNew.GetName() == buildPipelineConfigMapResourceName
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	})).Complete(r)
}

// Set Role for managing jobs/configmaps/secrets in the controller namespace

// +kubebuilder:rbac:namespace=system,groups=batch,resources=jobs,verbs=create;get;list;watch;delete;deletecollection
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch;update;delete;deletecollection
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;patch;update;delete;deletecollection

// +kubebuilder:rbac:groups=appstudio.redhat.com,resources=components,verbs=get;list

func (r *GitTektonResourcesRenovater) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("GitTektonResourcesRenovator")
	ctx = ctrllog.IntoContext(ctx, log)

	// Get Components
	componentList := &appstudiov1alpha1.ComponentList{}
	if err := r.client.List(ctx, componentList, &client.ListOptions{}); err != nil {
		log.Error(err, "failed to list Components", l.Action, l.ActionView)
		return ctrl.Result{}, err
	}
	var scmComponents []*git.ScmComponent
	for _, component := range componentList.Items {
		gitProvider, err := getGitProvider(component)
		if err != nil {
			// component misconfiguration shouldn't prevent other components from being updated
			// deepcopy the component to avoid implicit memory aliasing in for loop
			r.eventRecorder.Event(component.DeepCopy(), "Warning", "ErrorComponentProviderInfo", err.Error())
			continue
		}

		scmComponent, err := git.NewScmComponent(gitProvider, component.Spec.Source.GitSource.URL, component.Spec.Source.GitSource.Revision, component.Name, component.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
		scmComponents = append(scmComponents, scmComponent)
	}
	updateTargets := r.updateTargetProvider.GetUpdateTargets(ctx, scmComponents)
	log.V(l.DebugLevel).Info("found renovate targets", "targets", len(updateTargets))
	err := r.updateCoordinator.ExecuteUpdateWithLimits(ctx, renovate.UpdateTargetsToTektonDependeciesUpdate(updateTargets))
	if err != nil {
		log.Error(err, "failed to create a job", l.Action, l.ActionAdd)
	}
	return ctrl.Result{RequeueAfter: NextReconcile}, nil
}
