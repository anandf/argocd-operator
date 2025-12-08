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

// ArgoCDInstance is an interface that both ArgoCD and ClusterArgoCD implement.
// This allows shared reconciliation logic to work with both types polymorphically.
// +kubebuilder:object:generate=false
type ArgoCDInstance interface {
	// GetName returns the name of the ArgoCD instance
	GetName() string

	// GetNamespace returns the namespace for namespace-scoped instances
	// or the target deployment namespace for cluster-scoped instances
	GetNamespace() string

	// GetCommonSpec returns the common spec shared by both types
	GetCommonSpec() *ArgoCDCommonSpec

	// GetStatus returns the status
	GetStatus() *ArgoCDStatus

	// IsClusterScoped returns true if this is a ClusterArgoCD instance
	IsClusterScoped() bool

	// GetSourceNamespaces returns source namespaces for cross-namespace management
	GetSourceNamespaces() []string

	// GetObjectMeta returns the object metadata
	GetObjectMeta() *metav1.ObjectMeta

	// GetArgoCDAgent returns Agent configuration (nil for namespace-scoped)
	GetArgoCDAgent() *ArgoCDAgentSpec

	// Note: GetApplicationSet and GetNotifications are not included in this interface because they return
	// pointer types (*ArgoCDApplicationSet and *ArgoCDNotifications) which require nil handling.
	// Both ArgoCD and ClusterArgoCD share the same ArgoCDApplicationSet and ArgoCDNotifications types
	// (defined in ArgoCDCommonSpec). Access these fields directly via the concrete type's Spec field.
}
