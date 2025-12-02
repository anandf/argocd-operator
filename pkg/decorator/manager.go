package decorator

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

// DecoratorManager manages the application of decorators to Kubernetes objects
type DecoratorManager struct {
	decorators []Decorator
	logger     logr.Logger
}

// Decorator defines the interface for object decorators
type Decorator interface {
	Decorate(obj runtime.Object) error
}

// NewDecoratorManager creates a new DecoratorManager
func NewDecoratorManager(decorators ...Decorator) *DecoratorManager {
	return &DecoratorManager{
		decorators: decorators,
		logger:     logs.Log.WithName("DecoratorManager"),
	}
}

// AddDecorator adds a decorator to the manager
func (dm *DecoratorManager) AddDecorator(decorator Decorator) {
	dm.decorators = append(dm.decorators, decorator)
}

// Decorate applies all registered decorators to the given object
func (dm *DecoratorManager) Decorate(obj runtime.Object) error {
	if obj == nil {
		dm.logger.Info("skipping decoration of nil object")
		return nil
	}

	dm.logger.Info("applying decorators to object", "type", getObjectType(obj), "count", len(dm.decorators))

	for i, decorator := range dm.decorators {
		dm.logger.V(1).Info("applying decorator", "index", i, "decorator", getDecoratorName(decorator))
		if err := decorator.Decorate(obj); err != nil {
			dm.logger.Error(err, "failed to apply decorator", "index", i, "decorator", getDecoratorName(decorator))
			return err
		}
	}

	return nil
}

// DecorateAll applies all decorators to a list of objects
func (dm *DecoratorManager) DecorateAll(objects ...runtime.Object) error {
	for _, obj := range objects {
		if err := dm.Decorate(obj); err != nil {
			return err
		}
	}
	return nil
}

// getObjectType returns a string representation of the object type
func getObjectType(obj runtime.Object) string {
	if obj == nil {
		return "nil"
	}
	return obj.GetObjectKind().GroupVersionKind().String()
}

// getDecoratorName returns a string representation of the decorator type
func getDecoratorName(decorator Decorator) string {
	switch decorator.(type) {
	case *SCCDecorator:
		return "SCCDecorator"
	case *ResourceLimitsDecorator:
		return "ResourceLimitsDecorator"
	case *MonitoringDecorator:
		return "MonitoringDecorator"
	default:
		return "UnknownDecorator"
	}
}
