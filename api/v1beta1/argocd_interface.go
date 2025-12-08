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

// Ensure ArgoCD implements ArgoCDInstance interface at compile time
var _ ArgoCDInstance = &ArgoCD{}

// GetName returns the name of the ArgoCD instance
func (a *ArgoCD) GetName() string {
	return a.Name
}

// GetNamespace returns the namespace of the ArgoCD instance
func (a *ArgoCD) GetNamespace() string {
	return a.Namespace
}

// GetCommonSpec returns the common spec shared by both ArgoCD and ClusterArgoCD
func (a *ArgoCD) GetCommonSpec() *ArgoCDCommonSpec {
	return &a.Spec.ArgoCDCommonSpec
}

// GetStatus returns the status
func (a *ArgoCD) GetStatus() *ArgoCDStatus {
	return &a.Status
}

// IsClusterScoped returns false for namespace-scoped ArgoCD instances
func (a *ArgoCD) IsClusterScoped() bool {
	return false
}

// GetSourceNamespaces returns source namespaces (deprecated for namespace-scoped instances)
func (a *ArgoCD) GetSourceNamespaces() []string {
	// Return empty for namespace-scoped instances
	// The field exists but is deprecated
	return nil
}

// GetObjectMeta returns the object metadata
func (a *ArgoCD) GetObjectMeta() *metav1.ObjectMeta {
	return &a.ObjectMeta
}

// GetArgoCDAgent returns nil for namespace-scoped instances (not supported)
func (a *ArgoCD) GetArgoCDAgent() *ArgoCDAgentSpec {
	return nil
}
