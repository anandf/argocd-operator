/*
Copyright 2021.

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

package clusterargocd

import (
	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/controllers/shared"
)

// reconcileServices reconciles all Services for ClusterArgoCD components
func (r *ReconcileClusterArgoCD) reconcileServices(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling Services for ClusterArgoCD", "name", clusterArgoCD.Name)

	targetNamespace := getTargetNamespace(clusterArgoCD)

	// Reconcile server service
	if err := r.reconcileServerService(targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	// Reconcile repo-server service
	if err := r.reconcileRepoServerService(targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	// Reconcile application-controller metrics service
	if err := r.reconcileApplicationControllerMetricsService(targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	// Reconcile Redis service (if enabled)
	if err := r.reconcileRedisService(targetNamespace, clusterArgoCD); err != nil {
		return err
	}

	return nil
}

// reconcileServerService reconciles the ArgoCD Server service.
// This now delegates to the shared implementation to avoid code duplication
// with ArgoCD controller.
func (r *ReconcileClusterArgoCD) reconcileServerService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	return shared.ReconcileServerService(
		clusterArgoCD.Name,        // instanceName
		targetNamespace,           // namespace (uses ControlPlaneNamespace)
		clusterArgoCD.Spec.Server, // serverSpec (from embedded ArgoCDCommonSpec)
		clusterArgoCD,             // ownerRef (for garbage collection)
		r.Scheme,                  // scheme
		r.Client,                  // k8sClient
	)
}

// reconcileRepoServerService reconciles the ArgoCD Repo Server service.
// This now delegates to the shared implementation to avoid code duplication with ArgoCD controller.
func (r *ReconcileClusterArgoCD) reconcileRepoServerService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	return shared.ReconcileRepoServerService(
		clusterArgoCD.Name,      // instanceName
		targetNamespace,         // namespace
		clusterArgoCD.Spec.Repo, // repoSpec (from embedded ArgoCDCommonSpec)
		clusterArgoCD,           // ownerRef
		r.Scheme,                // scheme
		r.Client,                // k8sClient
	)
}

// reconcileApplicationControllerMetricsService reconciles the Application Controller metrics service.
// This now delegates to the shared implementation to avoid code duplication with ArgoCD controller.
func (r *ReconcileClusterArgoCD) reconcileApplicationControllerMetricsService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	return shared.ReconcileMetricsService(
		clusterArgoCD.Name, // instanceName
		targetNamespace,    // namespace
		clusterArgoCD,      // ownerRef
		r.Scheme,           // scheme
		r.Client,           // k8sClient
	)
}

// reconcileRedisService reconciles the Redis service
func (r *ReconcileClusterArgoCD) reconcileRedisService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling Redis service", "namespace", targetNamespace)

	// TODO: Implement Redis service creation/update
	log.Info("Redis service reconciliation not yet fully implemented")
	return nil
}
