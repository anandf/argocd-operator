package platform

import (
	"github.com/argoproj-labs/argocd-operator/pkg/component"
	"github.com/argoproj-labs/argocd-operator/pkg/decorator"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpenShift struct {
	client      client.Client
	scheme      *runtime.Scheme
	controllers ControllerMap
	decorators  DecoratorMap
}

func NewOpenShiftPlatform(c client.Client, scheme *runtime.Scheme) *OpenShift {
	o := &OpenShift{
		client:      c,
		scheme:      scheme,
		controllers: make(ControllerMap),
		decorators:  make(DecoratorMap),
	}

	// Initialize component controllers for OpenShift platform
	// OpenShift uses the same controllers as Kubernetes
	o.controllers["application-controller"] = component.NewApplicationController(c, scheme)
	o.controllers["application-set"] = component.NewApplicationSetController(c, scheme)
	o.controllers["server"] = component.NewServerController(c, scheme)
	o.controllers["repo-server"] = component.NewRepoServerController(c, scheme)
	o.controllers["redis"] = component.NewRedisController(c, scheme)
	o.controllers["dex"] = component.NewDexController(c, scheme)
	o.controllers["notifications"] = component.NewNotificationsController(c, scheme)

	// OpenShift-specific decorators for security and compliance
	o.decorators["scc"] = decorator.NewSCCDecorator(c, scheme)
	// Additional OpenShift decorators can be added here
	// o.decorators["route"] = decorator.NewRouteDecorator(c, scheme)
	// o.decorators["oauth"] = decorator.NewOAuthDecorator(c, scheme)

	return o
}

func (o *OpenShift) PlatformParams() PlatformConfig {
	controllerNames := make([]ControllerName, 0, len(o.controllers))
	for name := range o.controllers {
		controllerNames = append(controllerNames, name)
	}

	decoratorNames := make([]DecoratorName, 0, len(o.decorators))
	for name := range o.decorators {
		decoratorNames = append(decoratorNames, name)
	}

	return PlatformConfig{
		Name:            PlatformTypeOpenShift,
		ControllerNames: controllerNames,
		DecoratorNames:  decoratorNames,
	}
}

func (o *OpenShift) AllSupportedControllers() ControllerMap {
	return o.controllers
}

func (o *OpenShift) AllSupportedDecorators() DecoratorMap {
	return o.decorators
}
