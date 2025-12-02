package component

import (
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// MockDecorator for testing
type MockDecorator struct {
	Called    bool
	ShouldErr bool
}

func (m *MockDecorator) Decorate(obj runtime.Object) error {
	m.Called = true
	if m.ShouldErr {
		return fmt.Errorf("mock error")
	}
	return nil
}

// LabelDecorator adds a label to deployments
type LabelDecorator struct {
	Key   string
	Value string
}

func (d *LabelDecorator) Decorate(obj runtime.Object) error {
	switch typed := obj.(type) {
	case *appsv1.Deployment:
		if typed.Labels == nil {
			typed.Labels = make(map[string]string)
		}
		typed.Labels[d.Key] = d.Value
	}
	return nil
}

func TestNewDecoratorManager(t *testing.T) {
	manager := NewDecoratorManager()

	if manager == nil {
		t.Error("expected manager to be created")
	}
	if len(manager.decorators) != 0 {
		t.Errorf("expected empty decorators, got %d", len(manager.decorators))
	}
}

func TestNewDecoratorManagerWithDecorators(t *testing.T) {
	d1 := &MockDecorator{}
	d2 := &MockDecorator{}

	manager := NewDecoratorManager(d1, d2)

	if len(manager.decorators) != 2 {
		t.Errorf("expected 2 decorators, got %d", len(manager.decorators))
	}
}

func TestAddDecorator(t *testing.T) {
	manager := NewDecoratorManager()
	d := &MockDecorator{}

	manager.AddDecorator(d)

	if len(manager.decorators) != 1 {
		t.Errorf("expected 1 decorator, got %d", len(manager.decorators))
	}
}

func TestDecorateCallsAllDecorators(t *testing.T) {
	d1 := &MockDecorator{}
	d2 := &MockDecorator{}
	manager := NewDecoratorManager(d1, d2)

	deploy := &appsv1.Deployment{}
	err := manager.Decorate(deploy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !d1.Called {
		t.Error("expected d1 to be called")
	}
	if !d2.Called {
		t.Error("expected d2 to be called")
	}
}

func TestDecorateStopsOnError(t *testing.T) {
	d1 := &MockDecorator{ShouldErr: true}
	d2 := &MockDecorator{}
	manager := NewDecoratorManager(d1, d2)

	deploy := &appsv1.Deployment{}
	err := manager.Decorate(deploy)

	if err == nil {
		t.Error("expected error")
	}
	if !d1.Called {
		t.Error("expected d1 to be called")
	}
	if d2.Called {
		t.Error("expected d2 not to be called after error")
	}
}

func TestDecorateAppliesChanges(t *testing.T) {
	labelDecorator := &LabelDecorator{
		Key:   "test-label",
		Value: "test-value",
	}
	manager := NewDecoratorManager(labelDecorator)

	deploy := &appsv1.Deployment{}
	err := manager.Decorate(deploy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if deploy.Labels["test-label"] != "test-value" {
		t.Errorf("expected label 'test-value', got '%s'", deploy.Labels["test-label"])
	}
}

func TestDecorateWithMultipleDecorators(t *testing.T) {
	d1 := &LabelDecorator{Key: "label1", Value: "value1"}
	d2 := &LabelDecorator{Key: "label2", Value: "value2"}
	manager := NewDecoratorManager(d1, d2)

	deploy := &appsv1.Deployment{}
	err := manager.Decorate(deploy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if deploy.Labels["label1"] != "value1" {
		t.Errorf("expected label1='value1', got '%s'", deploy.Labels["label1"])
	}
	if deploy.Labels["label2"] != "value2" {
		t.Errorf("expected label2='value2', got '%s'", deploy.Labels["label2"])
	}
}

func TestDecorateWithEmptyList(t *testing.T) {
	manager := NewDecoratorManager()

	deploy := &appsv1.Deployment{}
	err := manager.Decorate(deploy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
