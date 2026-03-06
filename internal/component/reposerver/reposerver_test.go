package reposerver

import (
	"context"
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	comptesting "github.com/argoproj-labs/argocd-operator/internal/component/testing"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestRepoServerController_CreatesResources(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRepoServerController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify ServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, sa); err != nil {
		t.Errorf("expected ServiceAccount to be created: %v", err)
	}

	// Verify Deployment
	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, deploy); err != nil {
		t.Errorf("expected Deployment to be created: %v", err)
	}

	// Verify Service
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, svc); err != nil {
		t.Errorf("expected Service to be created: %v", err)
	}
}

func TestRepoServerController_Disabled_SkipsReconciliation(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithRepoDisabled())
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRepoServerController(c, scheme, nil)

	if controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return false when repo-server is disabled")
	}

	// Verify no Deployment exists
	deploy := &appsv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected Deployment to not exist when repo-server is disabled")
	}
}

func TestRepoServerController_Remote_SkipsReconciliation(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithRepoRemote("repo.example.com:8081"))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRepoServerController(c, scheme, nil)

	if controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return false when remote repo-server is configured")
	}

	// Verify no Deployment exists
	deploy := &appsv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected Deployment to not exist when remote repo-server is configured")
	}
}

func TestRepoServerController_CustomImage(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Image = "custom-argocd"
		a.Spec.Version = "v2.8.0"
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRepoServerController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	// Find the main container
	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name == "argocd-repo-server" {
			expectedImage := "custom-argocd:v2.8.0"
			if container.Image != expectedImage {
				t.Errorf("expected image %q, got %q", expectedImage, container.Image)
			}
			return
		}
	}
	t.Error("repo-server container not found in deployment")
}

func TestRepoServerController_CustomServiceAccount(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Repo.ServiceAccount = "custom-sa"
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRepoServerController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-repo-server", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if deploy.Spec.Template.Spec.ServiceAccountName != "custom-sa" {
		t.Errorf("expected service account 'custom-sa', got %q", deploy.Spec.Template.Spec.ServiceAccountName)
	}
}

func TestRepoServerController_Idempotent(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRepoServerController(c, scheme, nil)

	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
}

func TestRepoServerController_Name(t *testing.T) {
	controller := NewRepoServerController(nil, nil, nil)
	if controller.Name() != "repo-server" {
		t.Errorf("expected name 'repo-server', got %q", controller.Name())
	}
}
