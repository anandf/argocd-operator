package template_test

import (
	"testing"

	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"github.com/argoproj-labs/argocd-operator/pkg/component/template"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Example: Building template data for the server component
func ExampleTemplateData_server() {
	// Create a sample ArgoCD CR
	cr := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-argocd",
			Namespace: "argocd",
		},
		Spec: argoproj.ArgoCDSpec{
			Server: argoproj.ArgoCDServerSpec{
				Replicas: int32Ptr(2),
			},
		},
	}

	// Build template data
	data := template.NewTemplateData(cr, "argocd", "example-argocd", "server").
		WithLabels(map[string]string{
			"app.kubernetes.io/name":      "argocd-server",
			"app.kubernetes.io/instance":  "example-argocd",
			"app.kubernetes.io/component": "server",
		}).
		WithServiceAccount("example-argocd-server").
		WithImage("quay.io/argoproj/argocd:v2.9.0").
		WithExtra("Replicas", 2).
		WithExtra("ServiceType", "ClusterIP")

	// Data is now ready to be passed to template engine
	_ = data
}

// Example: Rendering a template
func ExampleTemplateEngine_RenderManifest() {
	// Note: In real usage, manifestsFS would be the embedded filesystem
	// This is just a demonstration of the API

	/*
		engine := template.NewTemplateEngine(manifestsFS, "manifests")

		cr := &argoproj.ArgoCD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-argocd",
				Namespace: "argocd",
			},
		}

		data := template.NewTemplateData(cr, "argocd", "example-argocd", "server").
			WithImage("quay.io/argoproj/argocd:v2.9.0").
			WithExtra("Replicas", 2)

		// Render a single template
		obj, err := engine.RenderManifest("base/server/deployment.yaml", data)
		if err != nil {
			log.Fatal(err)
		}

		// obj is now a client.Object (Unstructured) that can be created in the cluster
		// Convert to typed object if needed
		deployment := &appsv1.Deployment{}
		err = template.ConvertToTyped(obj, deployment)
	*/
}

// Example: Rendering all templates in a directory
func ExampleTemplateEngine_RenderManifests() {
	/*
		engine := template.NewTemplateEngine(manifestsFS, "manifests")

		data := template.NewTemplateData(cr, "argocd", "example-argocd", "server").
			WithImage("quay.io/argoproj/argocd:v2.9.0")

		// Render all templates in the server directory
		objs, err := engine.RenderManifests("base/server", data)
		if err != nil {
			log.Fatal(err)
		}

		// objs now contains all rendered resources:
		// - ServiceAccount
		// - Role
		// - RoleBinding
		// - Deployment
		// - Service
		// - Service (metrics)
	*/
}

// Test: Template data builder pattern
func TestTemplateDataBuilder(t *testing.T) {
	cr := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-argocd",
			Namespace: "test-ns",
		},
	}

	data := template.NewTemplateData(cr, "test-ns", "test-argocd", "server").
		WithLabels(map[string]string{
			"app": "argocd",
		}).
		WithAnnotations(map[string]string{
			"prometheus.io/scrape": "true",
		}).
		WithServiceAccount("test-sa").
		WithImage("test-image:latest").
		WithVersion("v2.9.0").
		WithExtra("Replicas", 3).
		WithExtra("ServiceType", "LoadBalancer")

	// Verify data
	assert.Equal(t, "test-ns", data.Namespace)
	assert.Equal(t, "test-argocd", data.Name)
	assert.Equal(t, "server", data.Component)
	assert.Equal(t, "argocd", data.Labels["app"])
	assert.Equal(t, "true", data.Annotations["prometheus.io/scrape"])
	assert.Equal(t, "test-sa", data.ServiceAccount)
	assert.Equal(t, "test-image:latest", data.Image)
	assert.Equal(t, "v2.9.0", data.Version)
	assert.Equal(t, 3, data.Extra["Replicas"])
	assert.Equal(t, "LoadBalancer", data.Extra["ServiceType"])
}

// Test: Fluent API chaining
func TestTemplateDataChaining(t *testing.T) {
	cr := &argoproj.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "ns",
		},
	}

	// All methods return *TemplateData, allowing chaining
	data := template.NewTemplateData(cr, "ns", "test", "component").
		WithLabels(map[string]string{"k1": "v1"}).
		WithLabels(map[string]string{"k2": "v2"}). // Can be called multiple times
		WithExtra("key1", "value1").
		WithExtra("key2", 123)

	assert.Equal(t, "v1", data.Labels["k1"])
	assert.Equal(t, "v2", data.Labels["k2"])
	assert.Equal(t, "value1", data.Extra["key1"])
	assert.Equal(t, 123, data.Extra["key2"])
}

// Test: Extra data types
func TestTemplateDataExtraTypes(t *testing.T) {
	cr := &argoproj.ArgoCD{}
	data := template.NewTemplateData(cr, "ns", "name", "comp")

	// Extra can hold any type
	data.
		WithExtra("string", "value").
		WithExtra("int", 42).
		WithExtra("bool", true).
		WithExtra("slice", []string{"a", "b", "c"}).
		WithExtra("map", map[string]string{"key": "value"})

	assert.Equal(t, "value", data.Extra["string"])
	assert.Equal(t, 42, data.Extra["int"])
	assert.Equal(t, true, data.Extra["bool"])
	assert.Equal(t, []string{"a", "b", "c"}, data.Extra["slice"])
	assert.Equal(t, "value", data.Extra["map"].(map[string]string)["key"])
}

// Helper function
func int32Ptr(i int32) *int32 {
	return &i
}
