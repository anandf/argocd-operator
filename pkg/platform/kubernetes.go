package platform

import (
	"github.com/argoproj-labs/argocd-operator/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Kubernetes struct {
	client      client.Client
	scheme      *runtime.Scheme
	controllers ControllerMap
	decorators  DecoratorMap
}

func NewKubernetesPlatform(c client.Client, scheme *runtime.Scheme) *Kubernetes {
	k := &Kubernetes{
		client:      c,
		scheme:      scheme,
		controllers: make(ControllerMap),
		decorators:  make(DecoratorMap),
	}

	// Initialize component controllers for Kubernetes platform
	k.controllers["application-controller"] = component.NewApplicationController(c, scheme)
	k.controllers["application-set"] = component.NewApplicationSetController(c, scheme)
	k.controllers["server"] = component.NewServerController(c, scheme)
	k.controllers["repo-server"] = component.NewRepoServerController(c, scheme)
	k.controllers["redis"] = component.NewRedisController(c, scheme)
	k.controllers["dex"] = component.NewDexController(c, scheme)
	k.controllers["notifications"] = component.NewNotificationsController(c, scheme)

	// Kubernetes platform has minimal decorators (no OpenShift-specific requirements)
	// Decorators can be added here if needed for Kubernetes-specific customizations

	return k
}

func (k *Kubernetes) PlatformParams() PlatformConfig {
	controllerNames := make([]ControllerName, 0, len(k.controllers))
	for name := range k.controllers {
		controllerNames = append(controllerNames, name)
	}

	decoratorNames := make([]DecoratorName, 0, len(k.decorators))
	for name := range k.decorators {
		decoratorNames = append(decoratorNames, name)
	}

	return PlatformConfig{
		Name:            PlatformTypeKubernetes,
		ControllerNames: controllerNames,
		DecoratorNames:  decoratorNames,
	}
}

func (k *Kubernetes) AllSupportedControllers() ControllerMap {
	return k.controllers
}

func (k *Kubernetes) AllSupportedDecorators() DecoratorMap {
	return k.decorators
}
