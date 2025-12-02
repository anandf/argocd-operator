package component

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

// Decorator is the interface for resource decorators
type Decorator interface {
	Decorate(obj runtime.Object) error
}

// DecoratorManager manages a list of decorators and applies them to objects
type DecoratorManager struct {
	decorators []Decorator
	logger     logr.Logger
}

// NewDecoratorManager creates a new decorator manager
func NewDecoratorManager(decorators ...Decorator) *DecoratorManager {
	return &DecoratorManager{
		decorators: decorators,
		logger:     logs.Log.WithName("DecoratorManager"),
	}
}

// AddDecorator adds a decorator to the manager
func (m *DecoratorManager) AddDecorator(decorator Decorator) {
	m.decorators = append(m.decorators, decorator)
}

// Decorate applies all decorators to the object
func (m *DecoratorManager) Decorate(obj runtime.Object) error {
	// Convert client.Object to runtime.Object if needed
	var runtimeObj runtime.Object
	if clientObj, ok := obj.(client.Object); ok {
		runtimeObj = clientObj.(runtime.Object)
	} else {
		runtimeObj = obj
	}

	// Apply each decorator
	for i, decorator := range m.decorators {
		m.logger.V(1).Info("applying decorator",
			"index", i,
			"decorator", fmt.Sprintf("%T", decorator))

		if err := decorator.Decorate(runtimeObj); err != nil {
			return fmt.Errorf("decorator %d (%T) failed: %w", i, decorator, err)
		}
	}

	return nil
}
