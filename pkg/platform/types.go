package platform

import (
	argoproj "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Platform defines a Kubernetes platform (Vanila Kubernetes, OpenShift...)
type Platform interface {
	PlatformParams() PlatformConfig
	AllSupportedControllers() ControllerMap
	AllSupportedDecorators() DecoratorMap
}

// PlatformConfig defines basic configuration that
// all platforms should support
type PlatformConfig struct {
	Name            string
	ControllerNames []ControllerName
	DecoratorNames  []DecoratorName
}

type Controller interface {
	Reconcile(cr *argoproj.ArgoCD) error
}

type Decorator interface {
	Decorate(object *runtime.Object) error
}

// ControllerName defines a name given to a controller(reconciler) in a platform
type ControllerName string
type DecoratorName string

// ControllerMap defines map that maps a name given to a controller(reconciler) to its injection.ControllerConstructor
type ControllerMap map[ControllerName]Controller
type DecoratorMap map[DecoratorName]Decorator
