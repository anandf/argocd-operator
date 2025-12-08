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

// ArgoCDDefaultNamespace is the default namespace for ClusterArgoCD components if not specified
const ArgoCDDefaultNamespace = "argocd"

// Define component names if not available in common package
const (
	ArgoCDRepoServerComponent              = "repo-server"
	ArgoCDNotificationsControllerComponent = "notifications-controller"
)

// getClusterArgoCDManagedByLabel returns the managed-by label value for ClusterArgoCD
// For ClusterArgoCD instances, the label value is the ClusterArgoCD name (not namespace)
// since ClusterArgoCD names are guaranteed to be unique cluster-wide.
func getClusterArgoCDManagedByLabel(clusterArgoCD *argoproj.ClusterArgoCD) string {
	return clusterArgoCD.Name
}

// getClusterArgoCDManagedByLabelKey returns the label key for managed-by-cluster-argocd
func getClusterArgoCDManagedByLabelKey() string {
	return common.ArgoCDManagedByClusterArgoCDLabel
}

// getApplicationSetManagedByLabelKey returns the label key for applicationset managed-by-cluster-argocd
func getApplicationSetManagedByLabelKey() string {
	return common.ArgoCDApplicationSetManagedByClusterArgoCDLabel
}

// getNotificationsManagedByLabelKey returns the label key for notifications managed-by-cluster-argocd
func getNotificationsManagedByLabelKey() string {
	return common.ArgoCDNotificationsManagedByClusterArgoCDLabel
}

// generateResourceName generates resource names for ClusterArgoCD components
// For cluster-scoped instances, we need to ensure unique names across the cluster
func generateResourceName(componentName string, clusterArgoCD *argoproj.ClusterArgoCD, targetNamespace string) string {
	// Format: <clusterargocd-name>-<target-namespace>-<component>
	// This ensures uniqueness when deploying resources across multiple namespaces
	return clusterArgoCD.Name + "-" + targetNamespace + "-" + componentName
}

// generateClusterRoleName generates ClusterRole names for ClusterArgoCD components
func generateClusterRoleName(componentName string, clusterArgoCD *argoproj.ClusterArgoCD) string {
	// Format: <clusterargocd-name>-<component>
	return clusterArgoCD.Name + "-" + componentName
}

// getTargetNamespace returns the control plane namespace where ClusterArgoCD components should be deployed
// This namespace contains namespace-scoped resources like Deployments, StatefulSets, ConfigMaps, Services, etc.
func getTargetNamespace(clusterArgoCD *argoproj.ClusterArgoCD) string {
	if clusterArgoCD.Spec.ControlPlaneNamespace != "" {
		return clusterArgoCD.Spec.ControlPlaneNamespace
	}
	// Default to "argocd" namespace
	return ArgoCDDefaultNamespace
}

// getServiceAccountName returns the ServiceAccount name for a component
func getServiceAccountName(componentName string, clusterArgoCD *argoproj.ClusterArgoCD) string {
	// Format: <clusterargocd-name>-<component>
	return clusterArgoCD.Name + "-" + componentName
}
