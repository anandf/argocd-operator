package dex

import (
	"context"
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	comptesting "github.com/argoproj-labs/argocd-operator/internal/component/testing"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDexController_Enabled_CreatesResources(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithSSO(argoproj.SSOProviderTypeDex))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewDexController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify ServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, sa); err != nil {
		t.Errorf("expected ServiceAccount to be created: %v", err)
	}

	// Verify Deployment
	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, deploy); err != nil {
		t.Errorf("expected Deployment to be created: %v", err)
	}

	// Verify Service
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, svc); err != nil {
		t.Errorf("expected Service to be created: %v", err)
	}
}

func TestDexController_Disabled_SkipsReconciliation(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	// No SSO configured = dex disabled
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewDexController(c, scheme, nil)

	if controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return false when SSO is not configured")
	}

	if err := controller.Remove(context.TODO(), cr); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify no Deployment exists
	deploy := &appsv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected Deployment to not exist when dex is disabled")
	}
}

func TestDexController_Cleanup(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithSSO(argoproj.SSOProviderTypeDex))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewDexController(c, scheme, nil)

	// First create resources
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify resources exist
	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("expected deployment to exist: %v", err)
	}

	// Disable dex and remove
	cr.Spec.SSO = nil
	if err := controller.Remove(context.TODO(), cr); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify resources are cleaned up
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected deployment to be deleted after cleanup")
	}
}

func TestDexController_CustomImage(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.SSO = &argoproj.ArgoCDSSOSpec{
			Provider: argoproj.SSOProviderTypeDex,
			Dex: &argoproj.ArgoCDDexSpec{
				Image:   "custom-dex",
				Version: "custom-version",
			},
		}
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewDexController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-dex-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	// Find the dex container (not init)
	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name == "dex" {
			expectedImage := "custom-dex:custom-version"
			if container.Image != expectedImage {
				t.Errorf("expected image %q, got %q", expectedImage, container.Image)
			}
			return
		}
	}
	t.Error("dex container not found in deployment")
}

func TestDexController_Idempotent(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithSSO(argoproj.SSOProviderTypeDex))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewDexController(c, scheme, nil)

	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
}

func TestDexController_Name(t *testing.T) {
	controller := NewDexController(nil, nil, nil)
	if controller.Name() != "dex" {
		t.Errorf("expected name 'dex', got %q", controller.Name())
	}
}
