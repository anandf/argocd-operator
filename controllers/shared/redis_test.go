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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestReconcileRedisStatefulSet_Creation tests creating a Redis HA StatefulSet
func TestReconcileRedisStatefulSet_Creation(t *testing.T) {
	// Setup test data
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: true,
	}

	redisSpec := argoproj.ArgoCDRedisSpec{}

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
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create fake client
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	// Call the shared function
	err := ReconcileRedisStatefulSet(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		false, // useTLS
		nil,   // applyHook
	)

	// Verify no error
	assert.NoError(t, err)

	// Verify StatefulSet was created
	ss := &appsv1.StatefulSet{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis-ha-server",
		Namespace: namespace,
	}, ss)

	assert.NoError(t, err, "StatefulSet should be created")
	assert.Equal(t, instanceName+"-redis-ha-server", ss.Name)
	assert.Equal(t, namespace, ss.Namespace)
	assert.Len(t, ss.Spec.Template.Spec.Containers, 2, "Should have redis and sentinel containers")
	assert.Equal(t, "redis", ss.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, "sentinel", ss.Spec.Template.Spec.Containers[1].Name)
	assert.Len(t, ss.Spec.Template.Spec.InitContainers, 1, "Should have config-init container")
}

// TestReconcileRedisStatefulSet_HADisabled tests that StatefulSet is not created when HA is disabled
func TestReconcileRedisStatefulSet_HADisabled(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: false, // HA disabled
	}

	redisSpec := argoproj.ArgoCDRedisSpec{}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisStatefulSet(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		false,
		nil,
	)

	assert.NoError(t, err)

	// Verify StatefulSet was NOT created
	ss := &appsv1.StatefulSet{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis-ha-server",
		Namespace: namespace,
	}, ss)

	assert.Error(t, err, "StatefulSet should not be created when HA is disabled")
}

// TestReconcileRedisStatefulSet_WithTLS tests creating StatefulSet with TLS enabled
func TestReconcileRedisStatefulSet_WithTLS(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: true,
	}

	redisSpec := argoproj.ArgoCDRedisSpec{}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisStatefulSet(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		true, // useTLS enabled
		nil,
	)

	assert.NoError(t, err)

	// Verify StatefulSet was created with TLS configuration
	ss := &appsv1.StatefulSet{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis-ha-server",
		Namespace: namespace,
	}, ss)

	assert.NoError(t, err)

	// Check sentinel container has TLS PostStart command
	sentinelContainer := ss.Spec.Template.Spec.Containers[1]
	assert.NotNil(t, sentinelContainer.Lifecycle)
	assert.NotNil(t, sentinelContainer.Lifecycle.PostStart)
	assert.NotNil(t, sentinelContainer.Lifecycle.PostStart.Exec)
	command := sentinelContainer.Lifecycle.PostStart.Exec.Command[2]
	assert.Contains(t, command, "--tls", "Sentinel PostStart should contain TLS flag")
}

// TestReconcileRedisDeployment_Creation tests creating a Redis Deployment
func TestReconcileRedisDeployment_Creation(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: false, // HA disabled, use non-HA deployment
	}

	redisSpec := argoproj.ArgoCDRedisSpec{}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisDeployment(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		false, // useTLS
		nil,
	)

	assert.NoError(t, err)

	// Verify Deployment was created
	deploy := &appsv1.Deployment{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis",
		Namespace: namespace,
	}, deploy)

	assert.NoError(t, err, "Deployment should be created")
	assert.Equal(t, instanceName+"-redis", deploy.Name)
	assert.Equal(t, namespace, deploy.Namespace)
	assert.Len(t, deploy.Spec.Template.Spec.Containers, 1, "Should have one redis container")
	assert.Equal(t, "redis", deploy.Spec.Template.Spec.Containers[0].Name)
}

// TestReconcileRedisDeployment_HAEnabled tests that Deployment is not created when HA is enabled
func TestReconcileRedisDeployment_HAEnabled(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: true, // HA enabled, should not create non-HA deployment
	}

	redisSpec := argoproj.ArgoCDRedisSpec{}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisDeployment(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		false,
		nil,
	)

	assert.NoError(t, err)

	// Verify Deployment was NOT created
	deploy := &appsv1.Deployment{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis",
		Namespace: namespace,
	}, deploy)

	assert.Error(t, err, "Deployment should not be created when HA is enabled")
}

// TestReconcileRedisDeployment_RedisDisabled tests that Deployment is not created when Redis is disabled
func TestReconcileRedisDeployment_RedisDisabled(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: false,
	}

	enabled := false
	redisSpec := argoproj.ArgoCDRedisSpec{
		Enabled: &enabled, // Redis disabled
	}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisDeployment(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		false,
		nil,
	)

	assert.NoError(t, err)

	// Verify Deployment was NOT created
	deploy := &appsv1.Deployment{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis",
		Namespace: namespace,
	}, deploy)

	assert.Error(t, err, "Deployment should not be created when Redis is disabled")
}

// TestReconcileRedisDeployment_WithTLS tests creating Deployment with TLS enabled
func TestReconcileRedisDeployment_WithTLS(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	haSpec := argoproj.ArgoCDHASpec{
		Enabled: false,
	}

	redisSpec := argoproj.ArgoCDRedisSpec{}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisDeployment(
		instanceName,
		namespace,
		haSpec,
		redisSpec,
		corev1.PullAlways,
		owner,
		scheme,
		k8sClient,
		true, // useTLS enabled
		nil,
	)

	assert.NoError(t, err)

	// Verify Deployment was created with TLS args
	deploy := &appsv1.Deployment{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis",
		Namespace: namespace,
	}, deploy)

	assert.NoError(t, err)

	// Check container has TLS args
	container := deploy.Spec.Template.Spec.Containers[0]
	argsStr := ""
	for _, arg := range container.Args {
		argsStr += arg + " "
	}
	assert.Contains(t, argsStr, "--tls-port", "Container args should contain TLS configuration")
}

// TestReconcileRedisNetworkPolicy_Creation tests creating a Redis NetworkPolicy
func TestReconcileRedisNetworkPolicy_Creation(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisNetworkPolicy(
		instanceName,
		namespace,
		nil, // no agent spec
		owner,
		scheme,
		k8sClient,
	)

	assert.NoError(t, err)

	// Verify NetworkPolicy was created
	np := &networkingv1.NetworkPolicy{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis-network-policy",
		Namespace: namespace,
	}, np)

	assert.NoError(t, err, "NetworkPolicy should be created")
	assert.Equal(t, instanceName+"-redis-network-policy", np.Name)
	assert.Equal(t, namespace, np.Namespace)

	// Verify ingress rules
	assert.Len(t, np.Spec.Ingress, 1, "Should have one ingress rule")
	assert.Len(t, np.Spec.Ingress[0].From, 3, "Should allow ingress from 3 components (controller, repo, server)")

	// Verify pod selector
	assert.Equal(t, instanceName+"-redis", np.Spec.PodSelector.MatchLabels["app.kubernetes.io/name"])
}

// TestReconcileRedisNetworkPolicy_WithAgents tests creating NetworkPolicy with agent specs
func TestReconcileRedisNetworkPolicy_WithAgents(t *testing.T) {
	instanceName := "test-argocd"
	namespace := "test-namespace"

	enabled := true
	agentSpec := &argoproj.ArgoCDAgentSpec{
		Principal: &argoproj.PrincipalSpec{
			Enabled: &enabled,
		},
		Agent: &argoproj.AgentSpec{
			Enabled: &enabled,
		},
	}

	owner := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName,
			Namespace: namespace,
			UID:       "test-uid",
		},
	}

	scheme := runtime.NewScheme()
	_ = argoproj.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		Build()

	err := ReconcileRedisNetworkPolicy(
		instanceName,
		namespace,
		agentSpec,
		owner,
		scheme,
		k8sClient,
	)

	assert.NoError(t, err)

	// Verify NetworkPolicy was created with agent peers
	np := &networkingv1.NetworkPolicy{}
	err = k8sClient.Get(nil, client.ObjectKey{
		Name:      instanceName + "-redis-network-policy",
		Namespace: namespace,
	}, np)

	assert.NoError(t, err)
	assert.Len(t, np.Spec.Ingress[0].From, 5, "Should allow ingress from 5 components (controller, repo, server, agent-principal, agent-agent)")
}

// TestGetRedisImage tests the Redis image helper function
func TestGetRedisImage(t *testing.T) {
	tests := []struct {
		name     string
		spec     argoproj.ArgoCDRedisSpec
		expected string
	}{
		{
			name:     "Default image and version",
			spec:     argoproj.ArgoCDRedisSpec{},
			expected: "public.ecr.aws/docker/library/redis@sha256:1a34bdba051ecd8a58ec8a3cc460acef697a1605e918149cc53d920673c1a0a7",
		},
		{
			name: "Custom image",
			spec: argoproj.ArgoCDRedisSpec{
				Image:   "custom-redis",
				Version: "7.0",
			},
			expected: "custom-redis:7.0",
		},
		{
			name: "Custom version only",
			spec: argoproj.ArgoCDRedisSpec{
				Version: "7.0",
			},
			expected: "public.ecr.aws/docker/library/redis:7.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRedisImage(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetRedisArgs tests the Redis args builder
func TestGetRedisArgs(t *testing.T) {
	tests := []struct {
		name     string
		useTLS   bool
		contains []string
	}{
		{
			name:   "Without TLS",
			useTLS: false,
			contains: []string{
				"--save",
				"--appendonly",
				"--requirepass",
			},
		},
		{
			name:   "With TLS",
			useTLS: true,
			contains: []string{
				"--save",
				"--appendonly",
				"--requirepass",
				"--tls-port",
				"--tls-cert-file",
				"--tls-key-file",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := getRedisArgs(tt.useTLS)
			argsStr := ""
			for _, arg := range args {
				argsStr += arg + " "
			}

			for _, expected := range tt.contains {
				assert.Contains(t, argsStr, expected, "Args should contain "+expected)
			}
		})
	}
}

// TestGetSentinelPostStartCommand tests the Sentinel PostStart command builder
func TestGetSentinelPostStartCommand(t *testing.T) {
	tests := []struct {
		name     string
		useTLS   bool
		contains string
	}{
		{
			name:     "Without TLS",
			useTLS:   false,
			contains: "redis-cli -p 26379 sentinel reset argocd",
		},
		{
			name:     "With TLS",
			useTLS:   true,
			contains: "redis-cli -p 26379 --tls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := getSentinelPostStartCommand(tt.useTLS)
			assert.Contains(t, cmd, tt.contains)
		})
	}
}

// TestGetRedisResourceRequirements tests resource requirements extraction
func TestGetRedisResourceRequirements(t *testing.T) {
	tests := []struct {
		name      string
		spec      argoproj.ArgoCDRedisSpec
		hasLimits bool
	}{
		{
			name:      "No resources specified",
			spec:      argoproj.ArgoCDRedisSpec{},
			hasLimits: false,
		},
		{
			name: "Resources specified",
			spec: argoproj.ArgoCDRedisSpec{
				Resources: &corev1.ResourceRequirements{},
			},
			hasLimits: false, // Empty resources
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := getRedisResourceRequirements(tt.spec)
			if tt.hasLimits {
				assert.NotNil(t, resources.Limits)
				assert.NotEmpty(t, resources.Limits)
			} else {
				assert.Empty(t, resources.Limits)
			}
		})
	}
}
