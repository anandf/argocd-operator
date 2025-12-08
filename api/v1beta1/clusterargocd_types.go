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

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// ClusterArgoCD is the Schema for the cluster-scoped argocds API
// ClusterArgoCD provides cluster-wide ArgoCD instance management with admin-specific features
// such as sourceNamespaces and argoCDAgent that are not available in namespace-scoped ArgoCD instances.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +operator-sdk:csv:customresourcedefinitions:resources={{ClusterArgoCD,v1beta1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ArgoCDExport,v1alpha1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ConfigMap,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{CronJob,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Ingress,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Job,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{PersistentVolumeClaim,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Pod,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Prometheus,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ReplicaSet,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Route,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Secret,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{Service,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{ServiceMonitor,v1,""}}
// +operator-sdk:csv:customresourcedefinitions:resources={{StatefulSet,v1,""}}
type ClusterArgoCD struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterArgoCDSpec `json:"spec,omitempty"`
	Status ArgoCDStatus      `json:"status,omitempty"`
}

// ClusterArgoCDSpec defines the desired state of ClusterArgoCD
// ClusterArgoCDSpec embeds ArgoCDCommonSpec and adds cluster-scoped specific fields
// like controlPlaneNamespace, sourceNamespaces, and argoCDAgent that are only available for cluster-scoped instances.
// +k8s:openapi-gen=true
type ClusterArgoCDSpec struct {
	// Embed all common fields shared with ArgoCD (includes ApplicationSet and Notifications)
	ArgoCDCommonSpec `json:",inline"`

	// ControlPlaneNamespace specifies the namespace where ClusterArgoCD control plane components will be deployed.
	// This includes namespace-scoped resources like Deployments, StatefulSets, ConfigMaps, Services, etc.
	// If not specified, defaults to "argocd".
	// The components deployed in this namespace will have cluster-wide permissions to manage
	// resources in sourceNamespaces.
	ControlPlaneNamespace string `json:"controlPlaneNamespace,omitempty"`

	// SourceNamespaces defines the namespaces where ArgoCD Applications, ApplicationSets,
	// and NotificationConfigurations are allowed to be created.
	// This field is only available in ClusterArgoCD and enables cross-namespace management
	// of all ArgoCD resources. This single field replaces the need for separate
	// sourceNamespaces fields in ApplicationSet and Notifications components.
	SourceNamespaces []string `json:"sourceNamespaces,omitempty"`

	// ArgoCDAgent defines configurations for the ArgoCD Agent component.
	// This field is only available in ClusterArgoCD for admin-level agent management.
	ArgoCDAgent *ArgoCDAgentSpec `json:"argoCDAgent,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterArgoCDList contains a list of ClusterArgoCD
type ClusterArgoCDList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterArgoCD `json:"items"`
}

// IsDeletionFinalizerPresent checks if the instance has deletion finalizer
func (clusterArgoCD *ClusterArgoCD) IsDeletionFinalizerPresent() bool {
	for _, finalizer := range clusterArgoCD.GetFinalizers() {
		if finalizer == "argoproj.io/finalizer" {
			return true
		}
	}
	return false
}

// ApplicationInstanceLabelKey returns either the custom application instance
// label key if set, or the default value.
func (c *ClusterArgoCD) ApplicationInstanceLabelKey() string {
	if c.Spec.ApplicationInstanceLabelKey != "" {
		return c.Spec.ApplicationInstanceLabelKey
	} else {
		return "app.kubernetes.io/instance"
	}
}
