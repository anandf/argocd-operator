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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Ensure ClusterArgoCD implements ArgoCDInstance interface at compile time
var _ ArgoCDInstance = &ClusterArgoCD{}

// GetName returns the name of the ClusterArgoCD instance
func (c *ClusterArgoCD) GetName() string {
	return c.Name
}

// GetNamespace returns the control plane namespace for cluster-scoped instances
func (c *ClusterArgoCD) GetNamespace() string {
	// For cluster-scoped instances, return the control plane namespace
	// where control plane components (Deployments, StatefulSets, ConfigMaps, etc.) are deployed
	if c.Spec.ControlPlaneNamespace != "" {
		return c.Spec.ControlPlaneNamespace
	}
	// Default to "argocd" namespace
	return "argocd"
}

// GetCommonSpec returns the common spec shared by both ArgoCD and ClusterArgoCD
func (c *ClusterArgoCD) GetCommonSpec() *ArgoCDCommonSpec {
	return &c.Spec.ArgoCDCommonSpec
}

// GetStatus returns the status
func (c *ClusterArgoCD) GetStatus() *ArgoCDStatus {
	return &c.Status
}

// IsClusterScoped returns true for cluster-scoped ClusterArgoCD instances
func (c *ClusterArgoCD) IsClusterScoped() bool {
	return true
}

// GetSourceNamespaces returns source namespaces for cross-namespace management
func (c *ClusterArgoCD) GetSourceNamespaces() []string {
	return c.Spec.SourceNamespaces
}

// GetObjectMeta returns the object metadata
func (c *ClusterArgoCD) GetObjectMeta() *metav1.ObjectMeta {
	return &c.ObjectMeta
}

// GetArgoCDAgent returns Agent configuration for cluster-scoped instances
func (c *ClusterArgoCD) GetArgoCDAgent() *ArgoCDAgentSpec {
	return c.Spec.ArgoCDAgent
}
