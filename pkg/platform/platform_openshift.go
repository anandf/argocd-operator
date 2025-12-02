//go:build openshift
// +build openshift

package platform

import (
	"github.com/argoproj-labs/argocd-operator/pkg/component"
	"github.com/argoproj-labs/argocd-operator/pkg/decorator"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type platformImpl struct {
	client      client.Client
	scheme      *runtime.Scheme
	controllers ControllerMap
	decorators  DecoratorMap
}

func NewPlatform(c client.Client, scheme *runtime.Scheme) Platform {
	p := &platformImpl{
		client:      c,
		scheme:      scheme,
		controllers: make(ControllerMap),
		decorators:  make(DecoratorMap),
	}

	// Initialize component controllers for OpenShift platform
	// OpenShift uses the same controllers as Kubernetes
	p.controllers["application-controller"] = component.NewApplicationController(c, scheme)
	p.controllers["application-set"] = component.NewApplicationSetController(c, scheme)
	p.controllers["server"] = component.NewServerController(c, scheme)
	p.controllers["repo-server"] = component.NewRepoServerController(c, scheme)
	p.controllers["redis"] = component.NewRedisController(c, scheme)
	p.controllers["dex"] = component.NewDexController(c, scheme)
	p.controllers["notifications"] = component.NewNotificationsController(c, scheme)

	// OpenShift-specific decorators for security and compliance
	p.decorators["scc"] = decorator.NewSCCDecorator(c, scheme)
	// Additional OpenShift decorators can be added here
	// p.decorators["route"] = decorator.NewRouteDecorator(c, scheme)
	// p.decorators["oauth"] = decorator.NewOAuthDecorator(c, scheme)

	return p
}

func (p *platformImpl) PlatformParams() PlatformConfig {
	controllerNames := make([]ControllerName, 0, len(p.controllers))
	for name := range p.controllers {
		controllerNames = append(controllerNames, name)
	}

	decoratorNames := make([]DecoratorName, 0, len(p.decorators))
	for name := range p.decorators {
		decoratorNames = append(decoratorNames, name)
	}

	return PlatformConfig{
		Name:            PlatformTypeOpenShift,
		ControllerNames: controllerNames,
		DecoratorNames:  decoratorNames,
	}
}

func (p *platformImpl) AllSupportedControllers() ControllerMap {
	return p.controllers
}

func (p *platformImpl) AllSupportedDecorators() DecoratorMap {
	return p.decorators
}
