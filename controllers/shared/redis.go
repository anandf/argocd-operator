// Copyright 2024 ArgoCD Operator Developers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shared

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcileRedisStatefulSet reconciles the Redis HA StatefulSet for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance
//   - namespace: Target namespace where the StatefulSet will be created
//   - haSpec: HA configuration from the spec (from ArgoCDCommonSpec)
//   - redisSpec: Redis configuration from the spec (from ArgoCDCommonSpec)
//   - imagePullPolicy: Image pull policy for containers
//   - ownerRef: Owner reference for garbage collection
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
//   - useTLS: Whether to enable TLS for Redis
//   - applyHook: Optional hook function to apply customizations
func ReconcileRedisStatefulSet(
	instanceName string,
	namespace string,
	haSpec argoproj.ArgoCDHASpec,
	redisSpec argoproj.ArgoCDRedisSpec,
	imagePullPolicy corev1.PullPolicy,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
	useTLS bool,
	applyHook func(interface{}, string) error,
) error {

	ssName := fmt.Sprintf("%s-redis-ha-server", instanceName)

	// Build the desired StatefulSet specification
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssName,
			Namespace: namespace,
			Labels:    makeLabelsForRedis(instanceName, "redis"),
		},
	}

	redisEnv := append(getProxyEnvVars(), corev1.EnvVar{
		Name: "AUTH",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-redis-initial-password", instanceName),
				},
				Key: "admin.password",
			},
		},
	})

	ss.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
	replicas := getRedisHAReplicas()
	ss.Spec.Replicas = replicas
	ss.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			common.ArgoCDKeyName: fmt.Sprintf("%s-redis-ha", instanceName),
		},
	}

	ss.Spec.ServiceName = fmt.Sprintf("%s-redis-ha", instanceName)

	ss.Spec.Template.ObjectMeta = metav1.ObjectMeta{
		Annotations: map[string]string{
			"checksum/init-config": "7128bfbb51eafaffe3c33b1b463e15f0cf6514cec570f9d9c4f2396f28c724ac",
		},
		Labels: map[string]string{
			common.ArgoCDKeyName: fmt.Sprintf("%s-redis-ha", instanceName),
		},
	}

	ss.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						common.ArgoCDKeyName: fmt.Sprintf("%s-redis-ha", instanceName),
					},
				},
				TopologyKey: common.ArgoCDKeyHostname,
			}},
		},
	}

	f := false
	ss.Spec.Template.Spec.AutomountServiceAccountToken = &f

	redisImage := getRedisHAImage(redisSpec)
	redisResources := getRedisHAResourceRequirements(haSpec)

	ss.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Args: []string{
				"/data/conf/redis.conf",
			},
			Command: []string{
				"redis-server",
			},
			Env:             redisEnv,
			Image:           redisImage,
			ImagePullPolicy: getImagePullPolicy(imagePullPolicy),
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/redis_liveness.sh",
						},
					},
				},
				FailureThreshold:    int32(5),
				InitialDelaySeconds: int32(30),
				PeriodSeconds:       int32(15),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(15),
			},
			Name: "redis",
			Ports: []corev1.ContainerPort{{
				ContainerPort: common.ArgoCDDefaultRedisPort,
				Name:          "redis",
			}},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/redis_readiness.sh",
						},
					},
				},
				FailureThreshold:    int32(5),
				InitialDelaySeconds: int32(30),
				PeriodSeconds:       int32(15),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(15),
			},
			Resources:       redisResources,
			SecurityContext: argoutil.DefaultSecurityContext(),
			VolumeMounts: []corev1.VolumeMount{
				{
					MountPath: "/data",
					Name:      "data",
				},
				{
					MountPath: "/health",
					Name:      "health",
				},
				{
					Name:      common.ArgoCDRedisServerTLSSecretName,
					MountPath: "/app/config/redis/tls",
				},
			},
		},
		{
			Args: []string{
				"/data/conf/sentinel.conf",
			},
			Command: []string{
				"redis-sentinel",
			},
			Env:             redisEnv,
			Image:           redisImage,
			ImagePullPolicy: getImagePullPolicy(imagePullPolicy),
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/sentinel_liveness.sh",
						},
					},
				},
				FailureThreshold:    int32(5),
				InitialDelaySeconds: int32(30),
				PeriodSeconds:       int32(15),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(15),
			},
			Name: "sentinel",
			Ports: []corev1.ContainerPort{{
				ContainerPort: common.ArgoCDDefaultRedisSentinelPort,
				Name:          "sentinel",
			}},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/sentinel_liveness.sh",
						},
					},
				},
				FailureThreshold:    int32(5),
				InitialDelaySeconds: int32(30),
				PeriodSeconds:       int32(15),
				SuccessThreshold:    int32(1),
				TimeoutSeconds:      int32(15),
			},
			Resources:       redisResources,
			SecurityContext: argoutil.DefaultSecurityContext(),
			Lifecycle: &corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"/bin/sh",
							"-c",
							getSentinelPostStartCommand(useTLS),
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					MountPath: "/data",
					Name:      "data",
				},
				{
					MountPath: "/health",
					Name:      "health",
				},
				{
					Name:      common.ArgoCDRedisServerTLSSecretName,
					MountPath: "/app/config/redis/tls",
				},
			},
		},
	}

	ss.Spec.Template.Spec.InitContainers = []corev1.Container{{
		Args: []string{
			"/readonly-config/init.sh",
		},
		Command: []string{
			"sh",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "SENTINEL_ID_0",
				Value: "3c0d9c0320bb34888c2df5757c718ce6ca992ce6",
			},
			{
				Name:  "SENTINEL_ID_1",
				Value: "40000915ab58c3fa8fd888fb8b24711944e6cbb4",
			},
			{
				Name:  "SENTINEL_ID_2",
				Value: "2bbec7894d954a8af3bb54d13eaec53cb024e2ca",
			},
			{
				Name: "AUTH",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: fmt.Sprintf("%s-redis-initial-password", instanceName),
						},
						Key: "admin.password",
					},
				},
			},
		},
		Image:           redisImage,
		ImagePullPolicy: getImagePullPolicy(imagePullPolicy),
		Name:            "config-init",
		Resources:       redisResources,
		SecurityContext: argoutil.DefaultSecurityContext(),
		VolumeMounts: []corev1.VolumeMount{
			{
				MountPath: "/readonly-config",
				Name:      "config",
				ReadOnly:  true,
			},
			{
				MountPath: "/data",
				Name:      "data",
			},
			{
				Name:      common.ArgoCDRedisServerTLSSecretName,
				MountPath: "/app/config/redis/tls",
			},
		},
	}}

	if isOpenShiftCluster() {
		var runAsNonRoot = true
		ss.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsNonRoot: &runAsNonRoot,
		}
	} else {
		var fsGroup int64 = 1000
		var runAsNonRoot = true
		var runAsUser int64 = 1000

		ss.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup:      &fsGroup,
			RunAsNonRoot: &runAsNonRoot,
			RunAsUser:    &runAsUser,
		}
	}

	addSeccompProfileForOpenShift(k8sClient, &ss.Spec.Template.Spec)

	ss.Spec.Template.Spec.ServiceAccountName = fmt.Sprintf("%s-argocd-redis-ha", instanceName)

	var terminationGracePeriodSeconds int64 = 60
	ss.Spec.Template.Spec.TerminationGracePeriodSeconds = &terminationGracePeriodSeconds

	var defaultMode int32 = 493
	ss.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: common.ArgoCDRedisHAConfigMapName,
					},
				},
			},
		},
		{
			Name: "health",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &defaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: common.ArgoCDRedisHAHealthConfigMapName,
					},
				},
			},
		},
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: common.ArgoCDRedisServerTLSSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: common.ArgoCDRedisServerTLSSecretName,
					Optional:   boolPtr(true),
				},
			},
		},
	}

	ss.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	}

	// Apply any custom hooks
	if applyHook != nil {
		if err := applyHook(ss, ""); err != nil {
			return err
		}
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ownerRef, ss, scheme); err != nil {
		return err
	}

	// Check if StatefulSet exists
	existingSS := &appsv1.StatefulSet{}
	ssExists, err := argoutil.IsObjectFound(k8sClient, namespace, ssName, existingSS)
	if err != nil {
		return err
	}

	if ssExists {
		// StatefulSet exists - determine if we should update or delete

		if !haSpec.Enabled || !redisSpec.IsEnabled() {
			// HA or Redis disabled - delete the StatefulSet
			var explanation string
			if !haSpec.Enabled {
				explanation = "ha is disabled"
			} else {
				explanation = "redis is disabled"
			}
			argoutil.LogResourceDeletion(log, existingSS, explanation)
			return k8sClient.Delete(context.TODO(), existingSS)
		}

		// Check if update is needed
		changed := false
		explanation := ""

		updateNodePlacementForStatefulSet(existingSS, ss, &changed, &explanation)

		for i, container := range existingSS.Spec.Template.Spec.Containers {
			if container.Image != redisImage {
				existingSS.Spec.Template.Spec.Containers[i].Image = redisImage
				existingSS.Spec.Template.Labels["image.upgraded"] = time.Now().UTC().Format("01022006-150406-MST")
				if changed {
					explanation += ", "
				}
				explanation += "container image"
				changed = true
			}
			if container.ImagePullPolicy != getImagePullPolicy(imagePullPolicy) {
				existingSS.Spec.Template.Spec.Containers[i].ImagePullPolicy = getImagePullPolicy(imagePullPolicy)
				if changed {
					explanation += ", "
				}
				explanation += "image pull policy"
				changed = true
			}
		}

		for i, container := range existingSS.Spec.Template.Spec.InitContainers {
			if container.Image != redisImage {
				existingSS.Spec.Template.Spec.InitContainers[i].Image = redisImage
				if changed {
					explanation += ", "
				}
				explanation += "init container image"
				changed = true
			}
			if container.ImagePullPolicy != getImagePullPolicy(imagePullPolicy) {
				existingSS.Spec.Template.Spec.InitContainers[i].ImagePullPolicy = getImagePullPolicy(imagePullPolicy)
				if changed {
					explanation += ", "
				}
				explanation += "init container image pull policy"
				changed = true
			}
		}

		if changed {
			argoutil.LogResourceUpdate(log, existingSS, "updating", explanation)
			return k8sClient.Update(context.TODO(), existingSS)
		}

		return nil // No update needed
	}

	// StatefulSet doesn't exist

	if !haSpec.Enabled || !redisSpec.IsEnabled() {
		return nil // HA or Redis disabled, don't create
	}

	argoutil.LogResourceCreation(log, ss)
	return k8sClient.Create(context.TODO(), ss)
}

// ReconcileRedisDeployment reconciles the Redis Deployment for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance
//   - namespace: Target namespace where the Deployment will be created
//   - haSpec: HA configuration from the spec (from ArgoCDCommonSpec)
//   - redisSpec: Redis configuration from the spec (from ArgoCDCommonSpec)
//   - imagePullPolicy: Image pull policy for containers
//   - ownerRef: Owner reference for garbage collection
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
//   - useTLS: Whether to enable TLS for Redis
//   - applyHook: Optional hook function to apply customizations
func ReconcileRedisDeployment(
	instanceName string,
	namespace string,
	haSpec argoproj.ArgoCDHASpec,
	redisSpec argoproj.ArgoCDRedisSpec,
	imagePullPolicy corev1.PullPolicy,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
	useTLS bool,
	applyHook func(interface{}, string) error,
) error {

	deployName := fmt.Sprintf("%s-redis", instanceName)

	// Build the desired Deployment specification
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: namespace,
			Labels:    makeLabelsForRedis(instanceName, "redis"),
		},
	}

	env := append(getProxyEnvVars(), corev1.EnvVar{
		Name: "REDIS_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-redis-initial-password", instanceName),
				},
				Key: "admin.password",
			},
		},
	})

	addSeccompProfileForOpenShift(k8sClient, &deploy.Spec.Template.Spec)

	if !isOpenShiftCluster() {
		deploy.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser: int64Ptr(1000),
		}
	}

	redisImage := getRedisImage(redisSpec)
	redisResources := getRedisResourceRequirements(redisSpec)

	deploy.Spec.Template.Spec.Containers = []corev1.Container{{
		Args:            getRedisArgs(useTLS),
		Image:           redisImage,
		ImagePullPolicy: getImagePullPolicy(imagePullPolicy),
		Name:            "redis",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: common.ArgoCDDefaultRedisPort,
			},
		},
		Resources:       redisResources,
		Env:             env,
		SecurityContext: argoutil.DefaultSecurityContext(),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      common.ArgoCDRedisServerTLSSecretName,
				MountPath: "/app/config/redis/tls",
			},
		},
	}}

	deploy.Spec.Template.Spec.ServiceAccountName = fmt.Sprintf("%s-argocd-redis", instanceName)
	deploy.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: common.ArgoCDRedisServerTLSSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: common.ArgoCDRedisServerTLSSecretName,
					Optional:   boolPtr(true),
				},
			},
		},
	}

	// Set selector
	deploy.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			common.ArgoCDKeyName: deployName,
		},
	}

	// Set template labels
	deploy.Spec.Template.ObjectMeta.Labels = map[string]string{
		common.ArgoCDKeyName: deployName,
	}

	// Apply any custom hooks
	if applyHook != nil {
		if err := applyHook(deploy, ""); err != nil {
			return err
		}
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ownerRef, deploy, scheme); err != nil {
		return err
	}

	// Check if Deployment exists
	existingDeploy := &appsv1.Deployment{}
	deplFound, err := argoutil.IsObjectFound(k8sClient, namespace, deployName, existingDeploy)
	if err != nil {
		return err
	}

	if deplFound {
		// Deployment exists - determine if we should update or delete

		if !redisSpec.IsEnabled() {
			// Redis disabled - delete the Deployment
			argoutil.LogResourceDeletion(log, deploy, "redis is disabled but deployment exists")
			return k8sClient.Delete(context.TODO(), deploy)
		} else if redisSpec.IsRemote() {
			argoutil.LogResourceDeletion(log, deploy, "remote redis is configured")
			return k8sClient.Delete(context.TODO(), deploy)
		}

		if haSpec.Enabled {
			// HA enabled - delete the non-HA Deployment
			argoutil.LogResourceDeletion(log, deploy, "redis ha is enabled but non-ha deployment exists")
			return k8sClient.Delete(context.TODO(), deploy)
		}

		// Check if update is needed
		changed := false
		explanation := ""

		actualImage := existingDeploy.Spec.Template.Spec.Containers[0].Image
		desiredImage := redisImage
		actualImagePullPolicy := existingDeploy.Spec.Template.Spec.Containers[0].ImagePullPolicy
		desiredImagePullPolicy := getImagePullPolicy(imagePullPolicy)

		if actualImage != desiredImage {
			existingDeploy.Spec.Template.Spec.Containers[0].Image = desiredImage
			existingDeploy.Spec.Template.Labels["image.upgraded"] = time.Now().UTC().Format("01022006-150406-MST")
			explanation = "container image"
			changed = true
		}

		if actualImagePullPolicy != desiredImagePullPolicy {
			existingDeploy.Spec.Template.Spec.Containers[0].ImagePullPolicy = desiredImagePullPolicy
			if changed {
				explanation += ", "
			}
			explanation += "image pull policy"
			changed = true
		}

		updateNodePlacementForDeployment(existingDeploy, deploy, &changed, &explanation)

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Args, existingDeploy.Spec.Template.Spec.Containers[0].Args) {
			existingDeploy.Spec.Template.Spec.Containers[0].Args = deploy.Spec.Template.Spec.Containers[0].Args
			if changed {
				explanation += ", "
			}
			explanation += "container args"
			changed = true
		}

		if !reflect.DeepEqual(existingDeploy.Spec.Template.Spec.Containers[0].Env,
			deploy.Spec.Template.Spec.Containers[0].Env) {
			existingDeploy.Spec.Template.Spec.Containers[0].Env = deploy.Spec.Template.Spec.Containers[0].Env
			if changed {
				explanation += ", "
			}
			explanation += "container env"
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].Resources, existingDeploy.Spec.Template.Spec.Containers[0].Resources) {
			existingDeploy.Spec.Template.Spec.Containers[0].Resources = deploy.Spec.Template.Spec.Containers[0].Resources
			if changed {
				explanation += ", "
			}
			explanation += "container resources"
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.Containers[0].SecurityContext, existingDeploy.Spec.Template.Spec.Containers[0].SecurityContext) {
			existingDeploy.Spec.Template.Spec.Containers[0].SecurityContext = deploy.Spec.Template.Spec.Containers[0].SecurityContext
			if changed {
				explanation += ", "
			}
			explanation += "container security context"
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.SecurityContext, existingDeploy.Spec.Template.Spec.SecurityContext) {
			existingDeploy.Spec.Template.Spec.SecurityContext = deploy.Spec.Template.Spec.SecurityContext
			if changed {
				explanation += ", "
			}
			explanation += "pod security context"
			changed = true
		}

		if !reflect.DeepEqual(deploy.Spec.Template.Spec.ServiceAccountName, existingDeploy.Spec.Template.Spec.ServiceAccountName) {
			existingDeploy.Spec.Template.Spec.ServiceAccountName = deploy.Spec.Template.Spec.ServiceAccountName
			if changed {
				explanation += ", "
			}
			explanation += "serviceAccountName"
			changed = true
		}

		if changed {
			argoutil.LogResourceUpdate(log, existingDeploy, "updating", explanation)
			return k8sClient.Update(context.TODO(), existingDeploy)
		}

		return nil // No update needed
	}

	// Deployment doesn't exist

	if redisSpec.IsEnabled() && redisSpec.IsRemote() {
		log.Info("Custom Redis Endpoint. Skipping starting redis.")
		return nil
	}

	if !redisSpec.IsEnabled() {
		log.Info("Redis disabled. Skipping starting redis.")
		return nil
	}

	if haSpec.Enabled {
		return nil // HA enabled, don't create non-HA deployment
	}

	argoutil.LogResourceCreation(log, deploy)
	return k8sClient.Create(context.TODO(), deploy)
}

// ReconcileRedisNetworkPolicy reconciles the Redis NetworkPolicy for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance
//   - namespace: Target namespace where the NetworkPolicy will be created
//   - agentSpec: ArgoCDAgent configuration (can be nil)
//   - ownerRef: Owner reference for garbage collection
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
func ReconcileRedisNetworkPolicy(
	instanceName string,
	namespace string,
	agentSpec *argoproj.ArgoCDAgentSpec,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
) error {

	npName := fmt.Sprintf("%s-redis-network-policy", instanceName)

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      npName,
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": fmt.Sprintf("%s-redis", instanceName),
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": fmt.Sprintf("%s-application-controller", instanceName),
								},
							},
						},
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": fmt.Sprintf("%s-repo-server", instanceName),
								},
							},
						},
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": fmt.Sprintf("%s-server", instanceName),
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: networkingProtocolPtr(corev1.ProtocolTCP),
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 6379},
						},
					},
				},
			},
		},
	}

	// Add agent-principal to allowed peers if enabled
	if agentSpec != nil && agentSpec.Principal != nil && agentSpec.Principal.IsEnabled() {
		networkPolicy.Spec.Ingress[0].From = append(networkPolicy.Spec.Ingress[0].From, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": fmt.Sprintf("%s-agent-principal", instanceName),
				},
			},
		})
	}

	// Add agent-agent to allowed peers if enabled
	if agentSpec != nil && agentSpec.Agent != nil && agentSpec.Agent.IsEnabled() {
		networkPolicy.Spec.Ingress[0].From = append(networkPolicy.Spec.Ingress[0].From, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": fmt.Sprintf("%s-agent-agent", instanceName),
				},
			},
		})
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ownerRef, networkPolicy, scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on redis network policy: %w", err)
	}

	// Check if NetworkPolicy exists
	existingNP := &networkingv1.NetworkPolicy{}
	npExists, err := argoutil.IsObjectFound(k8sClient, namespace, npName, existingNP)
	if err != nil {
		return err
	}

	if npExists {
		// NetworkPolicy exists - check if update is needed

		modified := false
		explanation := ""

		if !reflect.DeepEqual(existingNP.Spec.PodSelector, networkPolicy.Spec.PodSelector) {
			existingNP.Spec.PodSelector = networkPolicy.Spec.PodSelector
			explanation = "pod selector"
			modified = true
		}

		if !reflect.DeepEqual(existingNP.Spec.PolicyTypes, networkPolicy.Spec.PolicyTypes) {
			existingNP.Spec.PolicyTypes = networkPolicy.Spec.PolicyTypes
			if modified {
				explanation += ", "
			}
			explanation += "policy types"
			modified = true
		}

		if !reflect.DeepEqual(existingNP.Spec.Ingress, networkPolicy.Spec.Ingress) {
			existingNP.Spec.Ingress = networkPolicy.Spec.Ingress
			if modified {
				explanation += ", "
			}
			explanation += "ingress rules"
			modified = true
		}

		if modified {
			argoutil.LogResourceUpdate(log, existingNP, "updating", explanation)
			err := k8sClient.Update(context.TODO(), existingNP)
			if err != nil {
				log.Error(err, "Failed to update redis network policy")
				return fmt.Errorf("failed to update redis network policy: %w", err)
			}
		}

		return nil // No update needed
	}

	// NetworkPolicy doesn't exist - create it
	argoutil.LogResourceCreation(log, networkPolicy)
	if err := k8sClient.Create(context.TODO(), networkPolicy); err != nil {
		log.Error(err, "Failed to create redis network policy")
		return fmt.Errorf("failed to create redis network policy: %w", err)
	}

	return nil
}

// Helper functions

func makeLabelsForRedis(instanceName, component string) map[string]string {
	labels := common.DefaultLabels(instanceName)
	redisName := fmt.Sprintf("%s-%s", instanceName, component)
	labels[common.ArgoCDKeyName] = redisName
	labels[common.ArgoCDKeyComponent] = component
	return labels
}

func getRedisImage(redisSpec argoproj.ArgoCDRedisSpec) string {
	img := redisSpec.Image
	if img == "" {
		img = common.ArgoCDDefaultRedisImage
	}
	tag := redisSpec.Version
	if tag == "" {
		tag = common.ArgoCDDefaultRedisVersion
	}
	return argoutil.CombineImageTag(img, tag)
}

func getRedisHAImage(redisSpec argoproj.ArgoCDRedisSpec) string {
	img := redisSpec.Image
	if img == "" {
		img = common.ArgoCDDefaultRedisImage
	}
	tag := redisSpec.Version
	if tag == "" {
		tag = common.ArgoCDDefaultRedisVersionHA
	}
	return argoutil.CombineImageTag(img, tag)
}

func getRedisResourceRequirements(redisSpec argoproj.ArgoCDRedisSpec) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if redisSpec.Resources != nil {
		resources = *redisSpec.Resources
	}
	return resources
}

func getRedisHAResourceRequirements(haSpec argoproj.ArgoCDHASpec) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{}
	if haSpec.Resources != nil {
		resources = *haSpec.Resources
	}
	return resources
}

func getRedisHAReplicas() *int32 {
	replicas := common.ArgoCDDefaultRedisHAReplicas
	return &replicas
}

func getRedisArgs(useTLS bool) []string {
	args := make([]string, 0)
	args = append(args, "--save", "")
	args = append(args, "--appendonly", "no")
	args = append(args, "--requirepass $(REDIS_PASSWORD)")

	if useTLS {
		args = append(args, "--tls-port", "6379")
		args = append(args, "--port", "0")
		args = append(args, "--tls-cert-file", "/app/config/redis/tls/tls.crt")
		args = append(args, "--tls-key-file", "/app/config/redis/tls/tls.key")
		args = append(args, "--tls-auth-clients", "no")
	}

	return args
}

func getSentinelPostStartCommand(useTLS bool) string {
	if useTLS {
		return "sleep 30; redis-cli -p 26379 --tls --cert /app/config/redis/tls/tls.crt --key /app/config/redis/tls/tls.key --insecure sentinel reset argocd"
	}
	return "sleep 30; redis-cli -p 26379 sentinel reset argocd"
}

func getProxyEnvVars(vars ...corev1.EnvVar) []corev1.EnvVar {
	result := []corev1.EnvVar{}
	result = append(result, vars...)
	proxyKeys := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
	for _, p := range proxyKeys {
		if k, v := caseInsensitiveGetenv(p); k != "" {
			result = append(result, corev1.EnvVar{Name: k, Value: v})
		}
	}
	return result
}

func caseInsensitiveGetenv(s string) (string, string) {
	if v := os.Getenv(s); v != "" {
		return s, v
	}
	ls := strings.ToLower(s)
	if v := os.Getenv(ls); v != "" {
		return ls, v
	}
	return "", ""
}

func getImagePullPolicy(policy corev1.PullPolicy) corev1.PullPolicy {
	if policy == "" {
		return corev1.PullAlways
	}
	return policy
}

func boolPtr(val bool) *bool {
	return &val
}

func int64Ptr(val int64) *int64 {
	return &val
}

func networkingProtocolPtr(protocol corev1.Protocol) *corev1.Protocol {
	return &protocol
}

func isOpenShiftCluster() bool {
	// Check if OpenShift ClusterVersion API is available by trying to get it
	// This is a simple runtime check
	return isVersionAPIAvailable()
}

func isVersionAPIAvailable() bool {
	// Try to detect if running on OpenShift by checking if the config.openshift.io API group is available
	// This is a simplified version that checks at runtime
	// In practice, this would be initialized during controller setup
	// For now, we'll return false as a safe default
	// The actual check is done in cmd/main.go during initialization
	return false
}

func addSeccompProfileForOpenShift(k8sClient client.Client, podspec *corev1.PodSpec) {
	version, err := getClusterVersion(k8sClient)
	if err != nil {
		return
	}
	if version == "" {
		return
	}

	// Add seccomp profile for OpenShift 4.11+
	if version >= "4.11" {
		if podspec.SecurityContext == nil {
			podspec.SecurityContext = &corev1.PodSecurityContext{}
		}
		if podspec.SecurityContext.SeccompProfile == nil {
			podspec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			}
		}
	}
}

func getClusterVersion(k8sClient client.Client) (string, error) {
	clusterVersion := &configv1.ClusterVersion{}
	err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: "version"}, clusterVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	// Get version from status
	for _, condition := range clusterVersion.Status.History {
		if condition.State == configv1.CompletedUpdate {
			version := condition.Version
			if len(version) > 0 {
				// Extract major.minor version (e.g., "4.11.5" -> "4.11")
				parts := strings.Split(version, ".")
				if len(parts) >= 2 {
					return parts[0] + "." + parts[1], nil
				}
			}
		}
	}

	return "", nil
}

func updateNodePlacementForDeployment(existing *appsv1.Deployment, deploy *appsv1.Deployment, changed *bool, explanation *string) {
	if !reflect.DeepEqual(existing.Spec.Template.Spec.NodeSelector, deploy.Spec.Template.Spec.NodeSelector) {
		existing.Spec.Template.Spec.NodeSelector = deploy.Spec.Template.Spec.NodeSelector
		if *changed {
			*explanation += ", "
		}
		*explanation += "node selector"
		*changed = true
	}

	if !reflect.DeepEqual(existing.Spec.Template.Spec.Tolerations, deploy.Spec.Template.Spec.Tolerations) {
		existing.Spec.Template.Spec.Tolerations = deploy.Spec.Template.Spec.Tolerations
		if *changed {
			*explanation += ", "
		}
		*explanation += "tolerations"
		*changed = true
	}
}

func updateNodePlacementForStatefulSet(existing *appsv1.StatefulSet, ss *appsv1.StatefulSet, changed *bool, explanation *string) {
	if !reflect.DeepEqual(existing.Spec.Template.Spec.NodeSelector, ss.Spec.Template.Spec.NodeSelector) {
		existing.Spec.Template.Spec.NodeSelector = ss.Spec.Template.Spec.NodeSelector
		if *changed {
			*explanation += ", "
		}
		*explanation += "node selector"
		*changed = true
	}

	if !reflect.DeepEqual(existing.Spec.Template.Spec.Tolerations, ss.Spec.Template.Spec.Tolerations) {
		existing.Spec.Template.Spec.Tolerations = ss.Spec.Template.Spec.Tolerations
		if *changed {
			*explanation += ", "
		}
		*explanation += "tolerations"
		*changed = true
	}
}
