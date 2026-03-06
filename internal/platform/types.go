package platform

import (
	"github.com/argoproj-labs/argocd-operator/internal/component"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	PlatformTypeKubernetes = "kubernetes"
	PlatformTypeOpenShift  = "openshift"
)

// Well-known component names for ArgoCD components
const (
	ComponentNameRedis                 ComponentName = "redis"
	ComponentNameRepoServer            ComponentName = "repo-server"
	ComponentNameApplicationController ComponentName = "application-controller"
	ComponentNameServer                ComponentName = "server"
	ComponentNameApplicationSet        ComponentName = "application-set"
	ComponentNameDex                   ComponentName = "dex"
	ComponentNameNotifications         ComponentName = "notifications"
)

// DefaultReconcileOrder defines the order in which component controllers
// should be reconciled. The order respects component dependencies:
// Redis first (other components depend on it), then RepoServer
// (AppController and Server depend on it), then the rest.
var DefaultReconcileOrder = []ComponentName{
	ComponentNameRedis,
	ComponentNameRepoServer,
	ComponentNameApplicationController,
	ComponentNameServer,
	ComponentNameApplicationSet,
	ComponentNameDex,
	ComponentNameNotifications,
}

// Platform defines a Kubernetes platform (Vanilla Kubernetes, OpenShift...)
type Platform interface {
	PlatformParams() PlatformConfig
	AllComponents() ComponentMap
	AllSupportedDecorators() DecoratorMap
}

// PlatformConfig defines basic configuration that
// all platforms should support
type PlatformConfig struct {
	Name           string
	ComponentNames []ComponentName
	DecoratorNames []DecoratorName
}

// Decorator defines the interface for platform-specific object decorators
type Decorator interface {
	Decorate(object runtime.Object) error
}

// ComponentName defines a name given to a component in a platform
type ComponentName string
type DecoratorName string

// ComponentMap defines map that maps a component name to its Component
type ComponentMap map[ComponentName]component.Component
type DecoratorMap map[DecoratorName]Decorator
