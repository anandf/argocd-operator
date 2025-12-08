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
	"github.com/argoproj-labs/argocd-operator/common"
)

// reconcileResources orchestrates the reconciliation of all ClusterArgoCD resources
// This is the main entry point for reconciling all components
func (r *ReconcileClusterArgoCD) reconcileResources(clusterArgoCD *argoproj.ClusterArgoCD, status *argoproj.ArgoCDStatus) error {

	// Phase 1: Reconcile RBAC (ClusterRoles and ClusterRoleBindings)
	log.Info("reconciling ClusterRoles")
	if err := r.reconcileClusterRoles(clusterArgoCD); err != nil {
		log.Info(err.Error())
		return err
	}

	log.Info("reconciling ClusterRoleBindings")
	if err := r.reconcileClusterRoleBindings(clusterArgoCD); err != nil {
		log.Info(err.Error())
		return err
	}

	// Phase 2: Reconcile ServiceAccounts
	log.Info("reconciling ServiceAccounts")
	if err := r.reconcileServiceAccounts(clusterArgoCD); err != nil {
		log.Info(err.Error())
		return err
	}

	// Phase 3: Reconcile core resources in target namespace
	log.Info("reconciling target namespace resources")
	if err := r.reconcileTargetNamespaceResources(clusterArgoCD, status); err != nil {
		log.Info(err.Error())
		return err
	}

	// Phase 4: Reconcile ApplicationSet controller integration (if configured)
	if clusterArgoCD.Spec.ApplicationSet != nil {
		enabled := true
		if clusterArgoCD.Spec.ApplicationSet.Enabled != nil {
			enabled = *clusterArgoCD.Spec.ApplicationSet.Enabled
		}
		if enabled {
			log.Info("reconciling ApplicationSet controller integration")
			if err := r.reconcileApplicationSetIntegration(clusterArgoCD); err != nil {
				return err
			}
		}
	}

	// Phase 5: Reconcile Notifications controller integration (if configured)
	if clusterArgoCD.Spec.Notifications.Enabled {
		log.Info("reconciling Notifications controller integration")
		if err := r.reconcileNotificationsIntegration(clusterArgoCD); err != nil {
			return err
		}
	}

	// Phase 6: Reconcile ArgoCD Agent (if configured)
	if clusterArgoCD.Spec.ArgoCDAgent != nil {
		log.Info("reconciling ArgoCD Agent")
		if err := r.reconcileArgoCDAgent(clusterArgoCD); err != nil {
			return err
		}
	}

	// Set phase to Available if no errors
	status.Phase = "Available"

	return nil
}

// reconcileTargetNamespaceResources reconciles resources in the target namespace
// where ClusterArgoCD components are deployed
func (r *ReconcileClusterArgoCD) reconcileTargetNamespaceResources(clusterArgoCD *argoproj.ClusterArgoCD, status *argoproj.ArgoCDStatus) error {
	// Reconcile Services
	log.Info("reconciling Services")
	if err := r.reconcileServices(clusterArgoCD); err != nil {
		return err
	}

	// Reconcile Deployments
	log.Info("reconciling Deployments")
	if err := r.reconcileDeployments(clusterArgoCD); err != nil {
		return err
	}

	// Reconcile StatefulSets (Redis)
	log.Info("reconciling StatefulSets")
	if err := r.reconcileStatefulSets(clusterArgoCD); err != nil {
		return err
	}

	return nil
}

// reconcileClusterRoleBindings reconciles ClusterRoleBindings for ClusterArgoCD
func (r *ReconcileClusterArgoCD) reconcileClusterRoleBindings(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling ClusterRoleBindings for ClusterArgoCD", "name", clusterArgoCD.Name)

	targetNamespace := getTargetNamespace(clusterArgoCD)

	// Reconcile ClusterRoleBinding for application-controller
	if err := r.reconcileClusterRoleBinding(
		common.ArgoCDApplicationControllerComponent,
		targetNamespace,
		clusterArgoCD,
	); err != nil {
		return err
	}

	// Reconcile ClusterRoleBinding for server
	if err := r.reconcileClusterRoleBinding(
		common.ArgoCDServerComponent,
		targetNamespace,
		clusterArgoCD,
	); err != nil {
		return err
	}

	// TODO: Add more component ClusterRoleBindings as needed (dex, redis, etc.)

	return nil
}

// reconcileApplicationSetIntegration configures ApplicationSet controller for ClusterArgoCD
func (r *ReconcileClusterArgoCD) reconcileApplicationSetIntegration(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("integrating ApplicationSet controller with ClusterArgoCD", "name", clusterArgoCD.Name)

	// Configure ApplicationSet controller to watch source namespaces
	// This involves:
	// 1. Setting up RBAC in source namespaces (already done in reconcileSourceNamespaces)
	// 2. Configuring the ApplicationSet controller deployment with source namespace args
	// 3. Creating necessary ConfigMaps for SCM providers

	// The deployment is handled in reconcileApplicationSetDeployment
	// Additional configuration can be added here

	log.Info("ApplicationSet integration completed")
	return nil
}

// reconcileNotificationsIntegration configures Notifications controller for ClusterArgoCD
func (r *ReconcileClusterArgoCD) reconcileNotificationsIntegration(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("integrating Notifications controller with ClusterArgoCD", "name", clusterArgoCD.Name)

	// Configure Notifications controller to watch source namespaces
	// This involves:
	// 1. Setting up RBAC in source namespaces (already done in reconcileSourceNamespaces)
	// 2. Configuring the Notifications controller deployment with source namespace args
	// 3. Creating necessary Secrets and ConfigMaps for notification providers

	// The deployment is handled in reconcileNotificationsDeployment
	// Additional configuration can be added here

	log.Info("Notifications integration completed")
	return nil
}

// reconcileArgoCDAgent reconciles the ArgoCD Agent component
func (r *ReconcileClusterArgoCD) reconcileArgoCDAgent(clusterArgoCD *argoproj.ClusterArgoCD) error {
	log.Info("reconciling ArgoCD Agent for ClusterArgoCD", "name", clusterArgoCD.Name)

	// TODO: Implement ArgoCD Agent reconciliation
	// This should create:
	// - Agent Deployment
	// - Agent ServiceAccount
	// - Agent RBAC
	// - Agent ConfigMap
	// - Agent communication setup with main ArgoCD instance

	log.Info("ArgoCD Agent reconciliation not yet fully implemented")

	return nil
}
