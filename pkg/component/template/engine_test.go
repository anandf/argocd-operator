package template

import (
	"testing"
)

func TestTemplateDataCreation(t *testing.T) {
	data := NewTemplateData(nil, "test-ns", "test-name", "server")

	if data.Namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got '%s'", data.Namespace)
	}
	if data.Name != "test-name" {
		t.Errorf("expected name 'test-name', got '%s'", data.Name)
	}
	if data.Component != "server" {
		t.Errorf("expected component 'server', got '%s'", data.Component)
	}
	if data.Labels == nil {
		t.Error("expected Labels to be initialized")
	}
	if data.Annotations == nil {
		t.Error("expected Annotations to be initialized")
	}
	if data.Extra == nil {
		t.Error("expected Extra to be initialized")
	}
}

func TestWithLabels(t *testing.T) {
	data := NewTemplateData(nil, "ns", "name", "comp").
		WithLabels(map[string]string{"key1": "value1"}).
		WithLabels(map[string]string{"key2": "value2"})

	if data.Labels["key1"] != "value1" {
		t.Errorf("expected label key1='value1', got '%s'", data.Labels["key1"])
	}
	if data.Labels["key2"] != "value2" {
		t.Errorf("expected label key2='value2', got '%s'", data.Labels["key2"])
	}
}

func TestWithServiceAccount(t *testing.T) {
	data := NewTemplateData(nil, "ns", "name", "comp").
		WithServiceAccount("my-sa")

	if data.ServiceAccount != "my-sa" {
		t.Errorf("expected service account 'my-sa', got '%s'", data.ServiceAccount)
	}
}

func TestWithImage(t *testing.T) {
	data := NewTemplateData(nil, "ns", "name", "comp").
		WithImage("my-image:v1.0.0")

	if data.Image != "my-image:v1.0.0" {
		t.Errorf("expected image 'my-image:v1.0.0', got '%s'", data.Image)
	}
}

func TestWithExtra(t *testing.T) {
	data := NewTemplateData(nil, "ns", "name", "comp").
		WithExtra("key1", "value1").
		WithExtra("key2", 42)

	if data.Extra["key1"] != "value1" {
		t.Errorf("expected extra key1='value1', got '%v'", data.Extra["key1"])
	}
	if data.Extra["key2"] != 42 {
		t.Errorf("expected extra key2=42, got '%v'", data.Extra["key2"])
	}
}

func TestMethodChaining(t *testing.T) {
	data := NewTemplateData(nil, "ns", "name", "comp").
		WithLabels(map[string]string{"app": "test"}).
		WithServiceAccount("sa").
		WithImage("image:tag").
		WithExtra("replicas", 3)

	if data.Labels["app"] != "test" {
		t.Error("label not set correctly")
	}
	if data.ServiceAccount != "sa" {
		t.Error("service account not set correctly")
	}
	if data.Image != "image:tag" {
		t.Error("image not set correctly")
	}
	if data.Extra["replicas"] != 3 {
		t.Error("extra data not set correctly")
	}
}
