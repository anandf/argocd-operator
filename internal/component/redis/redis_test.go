package redis

import (
	"context"
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	comptesting "github.com/argoproj-labs/argocd-operator/internal/component/testing"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestRedisController_Standalone_CreatesResources(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify ServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, sa); err != nil {
		t.Errorf("expected ServiceAccount to be created: %v", err)
	}

	// Verify Deployment
	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy); err != nil {
		t.Errorf("expected Deployment to be created: %v", err)
	}

	// Verify Service
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, svc); err != nil {
		t.Errorf("expected Service to be created: %v", err)
	}
}

func TestRedisController_ExternalRedis_SkipsReconciliation(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	externalURL := "redis.external.example.com:6379"
	cr := comptesting.NewTestArgoCD(comptesting.WithExternalRedis(externalURL))
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)

	if controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return false when external Redis is configured")
	}

	if err := controller.Remove(context.TODO(), cr); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify no Deployment exists
	deploy := &appsv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected Deployment to not exist when external Redis is configured")
	}
}

func TestRedisController_Standalone_Idempotent(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)

	// Apply twice - should not error
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
}

func TestRedisController_Standalone_CustomImage(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Redis.Image = "custom-redis"
		a.Spec.Redis.Version = "custom-version"
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	expectedImage := "custom-redis:custom-version"
	if deploy.Spec.Template.Spec.Containers[0].Image != expectedImage {
		t.Errorf("expected image %q, got %q", expectedImage, deploy.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestRedisController_Standalone_CustomResources(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.Redis.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	resources := deploy.Spec.Template.Spec.Containers[0].Resources
	if resources.Requests == nil {
		t.Fatal("expected resource requests to be set")
	}
}

func TestRedisController_Standalone_NodePlacement(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(func(a *argoproj.ArgoCD) {
		a.Spec.NodePlacement = &argoproj.ArgoCDNodePlacementSpec{
			NodeSelector: map[string]string{"node-role": "infra"},
			Tolerations: []corev1.Toleration{
				{
					Key:      "node-role",
					Operator: corev1.TolerationOpEqual,
					Value:    "infra",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		}
	})
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if deploy.Spec.Template.Spec.NodeSelector["node-role"] != "infra" {
		t.Error("expected node selector to be applied")
	}
	if len(deploy.Spec.Template.Spec.Tolerations) == 0 {
		t.Error("expected tolerations to be applied")
	}
}

func TestRedisController_Cleanup(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD()
	c := comptesting.NewTestClient(scheme, cr)

	controller := NewRedisController(c, scheme, nil)

	// First create resources
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify resources exist
	deploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy); err != nil {
		t.Fatalf("expected deployment to exist: %v", err)
	}

	// Now set external redis and remove
	externalURL := "redis.external.example.com:6379"
	cr.Spec.Redis.Remote = &externalURL
	if err := controller.Remove(context.TODO(), cr); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify resources are cleaned up
	err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis", Namespace: "argocd"}, deploy)
	if err == nil {
		t.Error("expected deployment to be deleted after cleanup")
	}
}

func TestRedisController_HA_CreatesResources(t *testing.T) {
	scheme := comptesting.NewTestScheme()
	cr := comptesting.NewTestArgoCD(comptesting.WithHA(true))
	// Create the initial password secret that the templates reference
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-redis-initial-password",
			Namespace: "argocd",
		},
		Data: map[string][]byte{
			"admin.password": []byte("testpassword"),
		},
	}
	c := comptesting.NewTestClient(scheme, cr, secret)

	controller := NewRedisController(c, scheme, nil)
	if err := controller.Ensure(context.TODO(), cr); err != nil {
		t.Fatalf("Apply HA failed: %v", err)
	}

	// Verify HA ServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha", Namespace: "argocd"}, sa); err != nil {
		t.Errorf("expected HA ServiceAccount to be created: %v", err)
	}

	// Verify HA ConfigMap
	cm := &corev1.ConfigMap{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha-configmap", Namespace: "argocd"}, cm); err != nil {
		t.Errorf("expected HA ConfigMap to be created: %v", err)
	}

	// Verify HA Health ConfigMap
	healthCM := &corev1.ConfigMap{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha-health-configmap", Namespace: "argocd"}, healthCM); err != nil {
		t.Errorf("expected HA Health ConfigMap to be created: %v", err)
	}

	// Verify HA StatefulSet
	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha-server", Namespace: "argocd"}, sts); err != nil {
		t.Errorf("expected HA StatefulSet to be created: %v", err)
	}

	// Verify HA master service
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha", Namespace: "argocd"}, svc); err != nil {
		t.Errorf("expected HA master service to be created: %v", err)
	}

	// Verify HA announce service
	announceSvc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha-announce-0", Namespace: "argocd"}, announceSvc); err != nil {
		t.Errorf("expected HA announce service to be created: %v", err)
	}

	// Verify HAProxy service
	haproxySvc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha-haproxy", Namespace: "argocd"}, haproxySvc); err != nil {
		t.Errorf("expected HAProxy service to be created: %v", err)
	}

	// Verify HAProxy deployment
	haproxyDeploy := &appsv1.Deployment{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "argocd-redis-ha-haproxy", Namespace: "argocd"}, haproxyDeploy); err != nil {
		t.Errorf("expected HAProxy deployment to be created: %v", err)
	}
}

func TestRedisController_Name(t *testing.T) {
	controller := NewRedisController(nil, nil, nil)
	if controller.Name() != "redis" {
		t.Errorf("expected name 'redis', got %q", controller.Name())
	}
}

func TestRedisController_IsEnabled(t *testing.T) {
	controller := NewRedisController(nil, nil, nil)

	// Default: enabled
	cr := comptesting.NewTestArgoCD()
	if !controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return true for default CR")
	}

	// External Redis: disabled
	externalURL := "redis.external.example.com:6379"
	cr.Spec.Redis.Remote = &externalURL
	if controller.IsEnabled(cr) {
		t.Error("expected IsEnabled to return false when external Redis is configured")
	}
}
