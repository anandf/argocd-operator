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

// reconcileServerService reconciles the ArgoCD Server service
func (r *ReconcileClusterArgoCD) reconcileServerService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling Server service", "namespace", targetNamespace)

	// TODO: Implement server service creation/update
	// This should:
	// 1. Create Service with proper selector labels
	// 2. Configure HTTP/HTTPS ports
	// 3. Set up gRPC port for API
	// 4. Configure service type (ClusterIP, LoadBalancer, NodePort)

	log.Info("Server service reconciliation not yet fully implemented")
	return nil
}

// reconcileRepoServerService reconciles the ArgoCD Repo Server service
func (r *ReconcileClusterArgoCD) reconcileRepoServerService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling Repo Server service", "namespace", targetNamespace)

	// TODO: Implement repo-server service creation/update
	log.Info("Repo Server service reconciliation not yet fully implemented")
	return nil
}

// reconcileApplicationControllerMetricsService reconciles the Application Controller metrics service
func (r *ReconcileClusterArgoCD) reconcileApplicationControllerMetricsService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling Application Controller metrics service", "namespace", targetNamespace)

	// TODO: Implement application-controller metrics service creation/update
	log.Info("Application Controller metrics service reconciliation not yet fully implemented")
	return nil
}

// reconcileRedisService reconciles the Redis service
func (r *ReconcileClusterArgoCD) reconcileRedisService(targetNamespace string, clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling Redis service", "namespace", targetNamespace)

	// TODO: Implement Redis service creation/update
	log.Info("Redis service reconciliation not yet fully implemented")
	return nil
}
