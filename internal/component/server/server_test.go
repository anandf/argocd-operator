package server

import (
	"context"
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/internal/component"
	comptesting "github.com/argoproj-labs/argocd-operator/internal/component/testing"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestServerController_CreatesResources(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil, component.WithIngress())
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify ServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, sa); err != nil {
		t.Errorf("expected ServiceAccount to be created: %v", err)
	}

	// Verify Deployment
	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, deploy); err != nil {
		t.Errorf("expected Deployment to be created: %v", err)
	}

	// Verify Service
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, svc); err != nil {
		t.Errorf("expected Service to be created: %v", err)
	}

	// Verify Metrics Service
	metricsSvc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server-metrics", Namespace: "argocd"}, metricsSvc); err != nil {
		t.Errorf("expected Metrics Service to be created: %v", err)
	}
}

func TestServerController_Disabled_SkipsReconciliation(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithServerDisabled())
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil)

	if controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return false when server is disabled")
	}

	// Verify no Deployment exists
	deploy := &appsv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected Deployment to not exist when server is disabled")
	}
}

func TestServerController_CustomImage(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Image = "custom-argocd"
		a.Spec.Version = "v2.8.0"
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	expectedImage := "custom-argocd:v2.8.0"
	if deploy.Spec.Template.Spec.Containers[0].Image != expectedImage {
		t.Errorf("expected image %q, got %q", expectedImage, deploy.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestServerController_CustomReplicas(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	replicas := int32(3)
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Server.Replicas = &replicas
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %v", deploy.Spec.Replicas)
	}
}

func TestServerController_Insecure(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Server.Insecure = true
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	// Verify --insecure flag is in the command
	cmd := deploy.Spec.Template.Spec.Containers[0].Command
	found := false
	for _, arg := range cmd {
		if arg == "--insecure" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected --insecure flag in command")
	}
}

func TestServerController_Idempotent(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil)

	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
}

func TestServerController_Ingress_Creates(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithServerIngress(true))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil, component.WithIngress())
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	ingress := &networkingv1.Ingress{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, ingress); err != nil {
		t.Errorf("expected Ingress to be created: %v", err)
	}
}

func TestServerController_Ingress_Cleanup(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithServerIngress(true))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewServerController(c, scheme, nil, component.WithIngress())

	// First create resources with ingress enabled
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify ingress exists
	ingress := &networkingv1.Ingress{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, ingress); err != nil {
		t.Fatalf("expected ingress to exist: %v", err)
	}

	// Disable ingress and apply again
	cr.Spec.Server.Ingress.Enabled = false
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Cleanup apply failed: %v", err)
	}

	// Verify ingress is cleaned up
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-server", Namespace: "argocd"}, ingress)
	if err == nil {
		t.Error("expected ingress to be deleted after cleanup")
	}
}

func TestServerController_Name(t *testing.T) {
	controller := NewServerController(nil, nil, nil)
	if controller.Name() != "server" {
		t.Errorf("expected name 'server', got %q", controller.Name())
	}
}

func TestShortenHostname(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "short hostname unchanged",
			input:    "argocd.example.com",
			expected: "argocd.example.com",
		},
		{
			name:     "single label",
			input:    "argocd",
			expected: "argocd",
		},
		{
			name:  "long first label truncated",
			input: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example.com",
			// First label truncated to 63 chars
			expected: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := shortenHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("shortenHostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("shortenHostname() = %q, want %q", result, tt.expected)
			}
		})
	}
}
