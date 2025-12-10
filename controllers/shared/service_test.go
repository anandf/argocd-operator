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
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileServerService_Creation(t *testing.T) {
	// Setup test data
	instanceName := "test-argocd"
	namespace := "test-namespace"

	serverSpec := argoproj.ArgoCDServerSpec{
		Service: argoproj.ArgoCDServerServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Create a fake ArgoCD instance as owner reference
	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	// Setup scheme
	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	// Call the shared function
	err := ReconcileServerService(
		instanceName,
		namespace,
		serverSpec,
		owner,
		scheme,
		k8sClient,
	)

	// Verify no error
	assert.NoError(t, err)

	// Verify service was created
	svc := &corev1.Service{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-server",
		Namespace: namespace,
	}, svc)

	assert.NoError(t, err, "Service should be created")
	assert.Equal(t, instanceName+"-server", svc.Name)
	assert.Equal(t, namespace, svc.Namespace)
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Len(t, svc.Spec.Ports, 2, "Should have HTTP and HTTPS ports")
}

func TestReconcileServerService_Disabled(t *testing.T) {
	// Setup test data
	instanceName := "test-argocd"
	namespace := "test-namespace"

	// Server is disabled
	enabled := false
	serverSpec := argoproj.ArgoCDServerSpec{
		Enabled: &enabled,
		Service: argoproj.ArgoCDServerServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Create a fake ArgoCD instance as owner reference
	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	// Setup scheme
	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	// Call the shared function
	err := ReconcileServerService(
		instanceName,
		namespace,
		serverSpec,
		owner,
		scheme,
		k8sClient,
	)

	// Verify no error
	assert.NoError(t, err)

	// Verify service was NOT created
	svc := &corev1.Service{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-server",
		Namespace: namespace,
	}, svc)

	assert.Error(t, err, "Service should not be created when disabled")
}

func TestGetServiceType(t *testing.T) {
	tests := []struct {
		name     string
		spec     argoproj.ArgoCDServerSpec
		expected corev1.ServiceType
	}{
		{
			name: "ClusterIP specified",
			spec: argoproj.ArgoCDServerSpec{
				Service: argoproj.ArgoCDServerServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
			expected: corev1.ServiceTypeClusterIP,
		},
		{
			name: "LoadBalancer specified",
			spec: argoproj.ArgoCDServerSpec{
				Service: argoproj.ArgoCDServerServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
			},
			expected: corev1.ServiceTypeLoadBalancer,
		},
		{
			name:     "Default to ClusterIP",
			spec:     argoproj.ArgoCDServerSpec{},
			expected: corev1.ServiceTypeClusterIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getServiceType(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMakeLabelsForService(t *testing.T) {
	instanceName := "test-argocd"
	component := "server"

	labels := makeLabelsForService(instanceName, component)

	assert.NotNil(t, labels)
	assert.Contains(t, labels, "app.kubernetes.io/name")
	assert.Equal(t, "test-argocd-server", labels["app.kubernetes.io/name"])
	assert.Equal(t, "server", labels["app.kubernetes.io/component"])
}
