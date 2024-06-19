package renovate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tektonapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/konflux-ci/build-service/pkg/common"
	"github.com/konflux-ci/build-service/pkg/logs"
)

const (
	TargetsPerJob              = 20
	InstallationsPerJobEnvName = "RENOVATE_INSTALLATIONS_PER_JOB"
	TimeToLiveOfJob            = 24 * time.Hour
	RenovateImageEnvName       = "RENOVATE_IMAGE"
	DefaultRenovateImageUrl    = "quay.io/redhat-appstudio/renovate:v37.74.1"
)

// UpdateCoordinator is responsible for creating and managing renovate k8s jobs
type UpdateCoordinator struct {
	tasksPerJob      int
	renovateImageUrl string
	debug            bool
	client           client.Client
	scheme           *runtime.Scheme
}

func NewUpdateCoordinator(client client.Client, scheme *runtime.Scheme) *UpdateCoordinator {
	var targetsPerJobInt int
	targetsPerJobStr := os.Getenv(InstallationsPerJobEnvName)
	if regexp.MustCompile(`^\d{1,2}$`).MatchString(targetsPerJobStr) {
		targetsPerJobInt, _ = strconv.Atoi(targetsPerJobStr)
		if targetsPerJobInt == 0 {
			targetsPerJobInt = TargetsPerJob
		}
	} else {
		targetsPerJobInt = TargetsPerJob
	}
	renovateImageUrl := os.Getenv(RenovateImageEnvName)
	if renovateImageUrl == "" {
		renovateImageUrl = DefaultRenovateImageUrl
	}
	return &UpdateCoordinator{tasksPerJob: targetsPerJobInt, renovateImageUrl: renovateImageUrl, client: client, scheme: scheme, debug: false}
}

func (j *UpdateCoordinator) ExecuteUpdate(ctx context.Context, updates []*AuthenticatedUpdateConfig) error {

	if len(updates) == 0 {
		return nil
	}
	log := logger.FromContext(ctx)

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("renovate-job-%d-%s", timestamp, RandomString(5))
	log.Info(fmt.Sprintf("Creating renovate job %s for %d unique sets of scm repositories", name, len(updates)))

	secretTokens := map[string]string{}
	configMapData := map[string]string{}
	var renovateCmd []string
	for _, update := range updates {
		taskId := RandomString(5)
		secretTokens[taskId] = update.Token
		config, err := json.Marshal(update.Config)
		if err != nil {
			return err
		}
		configMapData[fmt.Sprintf("%s.json", taskId)] = string(config)

		log.Info("Creating renovate config map entry ", "json", config)
		renovateCmd = append(renovateCmd,
			fmt.Sprintf("RENOVATE_TOKEN=$TOKEN_%s RENOVATE_CONFIG_FILE=/configs/%s.json renovate || true", taskId, taskId),
		)
	}
	if len(renovateCmd) == 0 {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: BuildServiceNamespaceName,
		},
		StringData: secretTokens,
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: BuildServiceNamespaceName,
		},
		Data: configMapData,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: BuildServiceNamespaceName,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(TimeToLiveOfJob.Seconds())),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: name,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "renovate",
							Image: j.renovateImageUrl,
							EnvFrom: []corev1.EnvFromSource{
								{
									Prefix: "TOKEN_",
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: name,
										},
									},
								},
							},
							Command: []string{"bash", "-c", strings.Join(renovateCmd, "; ")},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      name,
									MountPath: "/configs",
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
								RunAsNonRoot:             ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if j.debug {
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
	}
	if err := j.client.Create(ctx, secret); err != nil {
		return err
	}
	if err := j.client.Create(ctx, configMap); err != nil {
		return err
	}
	if err := j.client.Create(ctx, job); err != nil {
		return err
	}
	log.Info("renovate job created", "jobname", job.Name, "updates", len(updates), logs.Action, logs.ActionAdd)
	if err := controllerutil.SetOwnerReference(job, secret, j.scheme); err != nil {
		return err
	}
	if err := j.client.Update(ctx, secret); err != nil {
		return err
	}

	if err := controllerutil.SetOwnerReference(job, configMap, j.scheme); err != nil {
		return err
	}
	if err := j.client.Update(ctx, configMap); err != nil {
		return err
	}
	return nil
}

func (j *UpdateCoordinator) ExecuteUpdateWithLimits(ctx context.Context, updates []*AuthenticatedUpdateConfig) error {
	for i := 0; i < len(updates); i += j.tasksPerJob {
		end := i + j.tasksPerJob

		if end > len(updates) {
			end = len(updates)
		}
		err := j.ExecuteUpdate(ctx, updates[i:end])
		if err != nil {
			return err
		}
	}
	return nil
}

// ExecutePipelineRun will create a renovate pipeline in the user namespace, to update component dependencies.
// The reasons for using a pipeline in the component namespace instead of a Job in the system namespace is as follows:
// - The user namespace has direct access to secrets to allow updating private images
// - Job's are removed after a timeout, so lots of nudges in a short period could make the namespace unusable due to pod Quota, while pipelines are pruned much more aggressively
// - Users can view the results of pipelines and the results are stored, making debugging much easier
// - Tekton automatically provides docker config from linked service accounts for private images, with a job I would need to implement this manually
//
// Warning: the installation token used here should only be scoped to the individual repositories being updated
func (j *UpdateCoordinator) ExecutePipelineRun(ctx context.Context, namespace string, updates []*AuthenticatedUpdateConfig) error {
	if len(updates) == 0 {
		return nil
	}
	timestamp := time.Now().Unix()
	log := logger.FromContext(ctx)
	name := fmt.Sprintf("renovate-pipeline-%d-%s", timestamp, RandomString(5))

	log.Info(fmt.Sprintf("Creating renovate pipeline %s for %d unique sets of scm repositories", name, len(updates)))
	secretTokens := map[string]string{}
	configMapData := map[string]string{}
	var renovateCmd []string
	for _, update := range updates {
		taskId := RandomString(5)
		secretTokens[taskId] = update.Token
		config, err := json.Marshal(update.Config)
		if err != nil {
			return err
		}
		configMapData[fmt.Sprintf("%s.json", taskId)] = string(config)

		log.Info(fmt.Sprintf("Creating renovate config map entry  value %s", config))
		renovateCmd = append(renovateCmd,
			fmt.Sprintf("RENOVATE_TOKEN=$TOKEN_%s RENOVATE_CONFIG_FILE=/configs/%s.json renovate || true", taskId, taskId),
		)
	}
	if len(renovateCmd) == 0 {
		return nil
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: secretTokens,
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: configMapData,
	}

	pipelineRun := &tektonapi.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: tektonapi.PipelineRunSpec{
			PipelineSpec: &tektonapi.PipelineSpec{
				Tasks: []tektonapi.PipelineTask{{
					Name: "renovate",
					TaskSpec: &tektonapi.EmbeddedTask{
						TaskSpec: tektonapi.TaskSpec{
							Steps: []tektonapi.Step{{
								Name:  "renovate",
								Image: j.renovateImageUrl,
								EnvFrom: []corev1.EnvFromSource{
									{
										Prefix: "TOKEN_",
										SecretRef: &corev1.SecretEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: name,
											},
										},
									},
								},
								Command: []string{"bash", "-c", strings.Join(renovateCmd, "; ")},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      name,
										MountPath: "/configs",
									},
								},
								SecurityContext: &corev1.SecurityContext{
									Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
									RunAsNonRoot:             ptr.To(true),
									AllowPrivilegeEscalation: ptr.To(false),
									SeccompProfile: &corev1.SeccompProfile{
										Type: corev1.SeccompProfileTypeRuntimeDefault,
									},
								},
							}},
							Volumes: []corev1.Volume{
								{
									Name: name,
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{Name: name},
										},
									},
								},
							},
						},
					},
				}},
			},
		},
	}
	if j.debug {
		pipelineRun.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps[0].Env = append(pipelineRun.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps[0].Env, corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
	}

	if err := j.client.Create(ctx, pipelineRun); err != nil {
		return err
	}
	// We create the PipelineRun first, and it will wait for the secret and configmap to be created
	if err := controllerutil.SetOwnerReference(pipelineRun, configMap, j.scheme); err != nil {
		return err
	}
	if err := controllerutil.SetOwnerReference(pipelineRun, secret, j.scheme); err != nil {
		return err
	}
	if err := j.client.Create(ctx, secret); err != nil {
		return err
	}
	if err := j.client.Create(ctx, configMap); err != nil {
		return err
	}
	log.Info("renovate Pipeline triggered", "pipeline", pipelineRun.Name, "tasks", len(updates), logs.Action, logs.ActionAdd)

	return nil
}
