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
	"reflect"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logr "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logr.Log.WithName("shared_reconcile")

// ReconcileServerService reconciles the ArgoCD Server service for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance (e.g., "argocd", "my-cluster-argocd")
//   - namespace: Target namespace where the service will be created
//   - serverSpec: Server configuration from the spec (from ArgoCDCommonSpec)
//   - ownerRef: Owner reference for garbage collection (ArgoCD or ClusterArgoCD)
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
//
// Returns:
//   - error: Any error encountered during reconciliation
func ReconcileServerService(
	instanceName string,
	namespace string,
	serverSpec argoproj.ArgoCDServerSpec,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
) error {

	// Generate the service name (e.g., "argocd-server")
	serviceName := fmt.Sprintf("%s-server", instanceName)

	// Build the desired service specification
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    makeLabelsForService(instanceName, "server"),
		},
	}

	// Handle AutoTLS annotation if needed
	wantsAutoTLS := serverSpec.WantsAutoTLS()
	if _, err := ensureAutoTLSAnnotation(k8sClient, svc, common.ArgoCDServerTLSSecretName, wantsAutoTLS); err != nil {
		return fmt.Errorf("unable to ensure AutoTLS annotation: %w", err)
	}

	// Configure service ports
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "http",
			Port:       80,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(8080),
		},
		{
			Name:       "https",
			Port:       443,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(8080),
		},
	}

	// Configure selector to match server pods
	svc.Spec.Selector = map[string]string{
		common.ArgoCDKeyName: serviceName,
	}

	// Configure service type (ClusterIP, LoadBalancer, NodePort, etc.)
	svc.Spec.Type = getServiceType(serverSpec)

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(ownerRef, svc, scheme); err != nil {
		return err
	}

	// Check if the service already exists
	existingSVC := &corev1.Service{}
	svcExists, err := argoutil.IsObjectFound(k8sClient, namespace, serviceName, existingSVC)
	if err != nil {
		return err
	}

	if svcExists {
		// Service exists - determine if we should update or delete it

		if !serverSpec.IsEnabled() {
			// Server is disabled - delete the service
			argoutil.LogResourceDeletion(log, svc, "argocd server is disabled")
			return k8sClient.Delete(context.TODO(), svc)
		}

		// Check if update is needed
		needsUpdate := false
		updateReason := ""

		// Check AutoTLS annotation
		update, err := ensureAutoTLSAnnotation(k8sClient, existingSVC, common.ArgoCDServerTLSSecretName, wantsAutoTLS)
		if err != nil {
			return err
		}
		if update {
			updateReason = "auto tls annotation"
			needsUpdate = true
		}

		// Check service type
		if !reflect.DeepEqual(svc.Spec.Type, existingSVC.Spec.Type) {
			existingSVC.Spec.Type = svc.Spec.Type
			if needsUpdate {
				updateReason += ", "
			}
			updateReason += "service type"
			needsUpdate = true
		}

		if needsUpdate {
			argoutil.LogResourceUpdate(log, existingSVC, "updating", updateReason)
			return k8sClient.Update(context.TODO(), existingSVC)
		}

		// No update needed
		return nil
	}

	// Service doesn't exist - create it if server is enabled

	if !serverSpec.IsEnabled() {
		return nil // Server disabled, don't create
	}

	argoutil.LogResourceCreation(log, svc)
	return k8sClient.Create(context.TODO(), svc)
}

// getServiceType extracts the service type from server spec with sensible default
func getServiceType(serverSpec argoproj.ArgoCDServerSpec) corev1.ServiceType {
	if len(serverSpec.Service.Type) > 0 {
		return serverSpec.Service.Type
	}
	return corev1.ServiceTypeClusterIP
}

// makeLabelsForService creates the standard labels for a service
func makeLabelsForService(instanceName, component string) map[string]string {
	labels := common.DefaultLabels(instanceName)
	serviceName := fmt.Sprintf("%s-%s", instanceName, component)
	labels[common.ArgoCDKeyName] = serviceName
	labels[common.ArgoCDKeyComponent] = component
	return labels
}

// ensureAutoTLSAnnotation ensures the AutoTLS annotation is set correctly on the service.
// This function is adapted from controllers/argocd/service.go
//
// Returns:
//   - bool: true if the annotation was updated
//   - error: any error encountered
func ensureAutoTLSAnnotation(k8sClient client.Client, svc *corev1.Service, secretName string, enabled bool) (bool, error) {
	var autoTLSAnnotationName, autoTLSAnnotationValue string

	// We currently only support OpenShift for automatic TLS
	if argoutil.IsRouteAPIAvailable() {
		autoTLSAnnotationName = common.AnnotationOpenShiftServiceCA
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		autoTLSAnnotationValue = secretName
	}

	if autoTLSAnnotationName != "" {
		val, ok := svc.Annotations[autoTLSAnnotationName]
		if enabled {
			// Don't request a TLS certificate from the OpenShift Service CA if the secret already exists.
			isTLSSecretFound, err := argoutil.IsObjectFound(k8sClient, svc.Namespace, secretName, &corev1.Secret{})
			if err != nil {
				return false, err
			}
			if !ok && isTLSSecretFound {
				log.Info(fmt.Sprintf("skipping AutoTLS on service %s since the TLS secret is already present", svc.Name))
				return false, nil
			}
			if !ok || val != secretName {
				log.Info(fmt.Sprintf("requesting AutoTLS on service %s", svc.Name))
				svc.Annotations[autoTLSAnnotationName] = autoTLSAnnotationValue
				return true, nil
			}
		} else {
			if ok {
				log.Info(fmt.Sprintf("removing AutoTLS on service %s", svc.Name))
				delete(svc.Annotations, autoTLSAnnotationName)
				return true, nil
			}
		}
	}

	return false, nil
}

// ReconcileRepoServerService reconciles the ArgoCD Repo Server service for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance
//   - namespace: Target namespace where the service will be created
//   - repoSpec: Repo server configuration from the spec (from ArgoCDCommonSpec)
//   - ownerRef: Owner reference for garbage collection
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
func ReconcileRepoServerService(
	instanceName string,
	namespace string,
	repoSpec argoproj.ArgoCDRepoSpec,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
) error {

	serviceName := fmt.Sprintf("%s-repo-server", instanceName)

	// Build the desired service specification
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    makeLabelsForService(instanceName, "repo-server"),
		},
	}

	// Handle AutoTLS annotation
	wantsAutoTLS := repoSpec.WantsAutoTLS()
	if _, err := ensureAutoTLSAnnotation(k8sClient, svc, common.ArgoCDRepoServerTLSSecretName, wantsAutoTLS); err != nil {
		return fmt.Errorf("unable to ensure AutoTLS annotation: %w", err)
	}

	// Configure service type
	svc.Spec.Type = corev1.ServiceTypeClusterIP

	// Configure ports (server and metrics)
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "server",
			Port:       common.ArgoCDDefaultRepoServerPort,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(common.ArgoCDDefaultRepoServerPort),
		},
		{
			Name:       "metrics",
			Port:       common.ArgoCDDefaultRepoMetricsPort,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(common.ArgoCDDefaultRepoMetricsPort),
		},
	}

	// Configure selector
	svc.Spec.Selector = map[string]string{
		common.ArgoCDKeyName: serviceName,
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ownerRef, svc, scheme); err != nil {
		return err
	}

	// Check if service exists
	existingSVC := &corev1.Service{}
	svcExists, err := argoutil.IsObjectFound(k8sClient, namespace, serviceName, existingSVC)
	if err != nil {
		return err
	}

	if svcExists {
		// Service exists - determine if we should update or delete

		if !repoSpec.IsEnabled() {
			// Repo server disabled - delete the service
			argoutil.LogResourceDeletion(log, svc, "repo server is disabled")
			return k8sClient.Delete(context.TODO(), svc)
		}

		// Check if remote repo is configured (should delete local service)
		if repoSpec.IsRemote() {
			argoutil.LogResourceDeletion(log, svc, "remote repo server is configured")
			return k8sClient.Delete(context.TODO(), svc)
		}

		// Check AutoTLS annotation update
		update, err := ensureAutoTLSAnnotation(k8sClient, existingSVC, common.ArgoCDRepoServerTLSSecretName, wantsAutoTLS)
		if err != nil {
			return err
		}
		if update {
			argoutil.LogResourceUpdate(log, existingSVC, "updating auto tls annotation")
			return k8sClient.Update(context.TODO(), existingSVC)
		}

		return nil // No update needed
	}

	// Service doesn't exist

	if !repoSpec.IsEnabled() {
		return nil // Repo server disabled, don't create
	}

	// Don't create local service if remote repo is configured
	if repoSpec.IsRemote() {
		log.Info("skip creating repo server service, repo remote is enabled")
		return nil
	}

	argoutil.LogResourceCreation(log, svc)
	return k8sClient.Create(context.TODO(), svc)
}

// ReconcileRedisService reconciles the Redis service for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance
//   - namespace: Target namespace where the service will be created
//   - redisSpec: Redis configuration from the spec (from ArgoCDCommonSpec)
//   - ownerRef: Owner reference for garbage collection
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
func ReconcileRedisService(
	instanceName string,
	namespace string,
	redisSpec argoproj.ArgoCDRedisSpec,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
) error {

	serviceName := fmt.Sprintf("%s-redis", instanceName)

	// Build the desired service specification
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    makeLabelsForService(instanceName, "redis"),
		},
	}

	// Handle AutoTLS annotation
	wantsAutoTLS := redisSpec.WantsAutoTLS()
	if _, err := ensureAutoTLSAnnotation(k8sClient, svc, common.ArgoCDRedisServerTLSSecretName, wantsAutoTLS); err != nil {
		return fmt.Errorf("unable to ensure AutoTLS annotation: %w", err)
	}

	// Configure service type
	svc.Spec.Type = corev1.ServiceTypeClusterIP

	// Configure ports
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "tcp-redis",
			Port:       common.ArgoCDDefaultRedisPort,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(common.ArgoCDDefaultRedisPort),
		},
	}

	// Configure selector
	svc.Spec.Selector = map[string]string{
		common.ArgoCDKeyName: serviceName,
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ownerRef, svc, scheme); err != nil {
		return err
	}

	// Check if service exists
	existingSVC := &corev1.Service{}
	svcExists, err := argoutil.IsObjectFound(k8sClient, namespace, serviceName, existingSVC)
	if err != nil {
		return err
	}

	if svcExists {
		// Service exists - determine if we should update or delete

		if !redisSpec.IsEnabled() {
			// Redis disabled - delete the service
			argoutil.LogResourceDeletion(log, svc, "redis is disabled")
			return k8sClient.Delete(context.TODO(), svc)
		}

		// Check AutoTLS annotation update
		update, err := ensureAutoTLSAnnotation(k8sClient, existingSVC, common.ArgoCDRedisServerTLSSecretName, wantsAutoTLS)
		if err != nil {
			return err
		}
		if update {
			argoutil.LogResourceUpdate(log, existingSVC, "updating auto tls annotation")
			return k8sClient.Update(context.TODO(), existingSVC)
		}

		return nil // No update needed
	}

	// Service doesn't exist

	if !redisSpec.IsEnabled() {
		return nil // Redis disabled, don't create
	}

	argoutil.LogResourceCreation(log, svc)
	return k8sClient.Create(context.TODO(), svc)
}

// ReconcileMetricsService reconciles the Application Controller metrics service for any ArgoCD instance.
// This shared function works for both namespace-scoped ArgoCD and cluster-scoped ClusterArgoCD.
//
// Parameters:
//   - instanceName: Name of the ArgoCD instance
//   - namespace: Target namespace where the service will be created
//   - ownerRef: Owner reference for garbage collection
//   - scheme: Kubernetes scheme for setting owner references
//   - k8sClient: Kubernetes client for CRUD operations
func ReconcileMetricsService(
	instanceName string,
	namespace string,
	ownerRef metav1.Object,
	scheme *runtime.Scheme,
	k8sClient client.Client,
) error {

	serviceName := fmt.Sprintf("%s-metrics", instanceName)
	appControllerName := fmt.Sprintf("%s-application-controller", instanceName)

	// Build the desired service specification
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    makeLabelsForService(instanceName, "metrics"),
		},
	}

	// Configure service type
	svc.Spec.Type = corev1.ServiceTypeClusterIP

	// Configure ports
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "metrics",
			Port:       8082,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(8082),
		},
	}

	// Configure selector (points to application-controller)
	svc.Spec.Selector = map[string]string{
		common.ArgoCDKeyName: appControllerName,
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(ownerRef, svc, scheme); err != nil {
		return err
	}

	// Check if service exists
	existingSVC := &corev1.Service{}
	svcExists, err := argoutil.IsObjectFound(k8sClient, namespace, serviceName, existingSVC)
	if err != nil {
		return err
	}

	if svcExists {
		// Service exists - nothing to update for metrics service
		// (it's a simple service with no configurable options)
		return nil
	}

	// Service doesn't exist - create it
	argoutil.LogResourceCreation(log, svc)
	return k8sClient.Create(context.TODO(), svc)
}
