//go:build openshift
// +build openshift

package platform

import (
	"github.com/argoproj-labs/argocd-operator/internal/component"
	"github.com/argoproj-labs/argocd-operator/internal/component/appcontroller"
	"github.com/argoproj-labs/argocd-operator/internal/component/appsetcontroller"
	"github.com/argoproj-labs/argocd-operator/internal/component/dex"
	"github.com/argoproj-labs/argocd-operator/internal/component/notifications"
	"github.com/argoproj-labs/argocd-operator/internal/component/redis"
	"github.com/argoproj-labs/argocd-operator/internal/component/reposerver"
	"github.com/argoproj-labs/argocd-operator/internal/component/server"
	"github.com/argoproj-labs/argocd-operator/internal/decorator"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type platformImpl struct {
	client     client.Client
	scheme     *runtime.Scheme
	components ComponentMap
	decorators DecoratorMap
}

func NewPlatform(c client.Client, scheme *runtime.Scheme) Platform {
	p := &platformImpl{
		client:     c,
		scheme:     scheme,
		components: make(ComponentMap),
		decorators: make(DecoratorMap),
	}

	// Initialize core ArgoCD component controllers for OpenShift platform.
	// OpenShift supports the same core controllers as Kubernetes.
	// The Server component gets WithRoute() since OpenShift uses Route resources.

	// OpenShift-specific decorators for security and compliance
	sccDecorator := decorator.NewSCCDecorator(c, scheme)
	p.decorators["scc"] = sccDecorator

	// Build a DecoratorManager with OpenShift-specific decorators for component controllers
	componentDecorators := decorator.NewDecoratorManager(sccDecorator)
	p.components[ComponentNameRedis] = redis.NewRedisController(c, scheme, componentDecorators)
	p.components[ComponentNameRepoServer] = reposerver.NewRepoServerController(c, scheme, componentDecorators)
	p.components[ComponentNameApplicationController] = appcontroller.NewApplicationController(c, scheme, componentDecorators)
	p.components[ComponentNameServer] = server.NewServerController(c, scheme, componentDecorators, component.WithRoute())
	p.components[ComponentNameApplicationSet] = appsetcontroller.NewApplicationSetController(c, scheme, componentDecorators)
	p.components[ComponentNameDex] = dex.NewDexController(c, scheme, componentDecorators)
	p.components[ComponentNameNotifications] = notifications.NewNotificationsController(c, scheme, componentDecorators)

	return p
}

func (p *platformImpl) PlatformParams() PlatformConfig {
	componentNames := make([]ComponentName, 0, len(p.components))
	for name := range p.components {
		componentNames = append(componentNames, name)
	}

	decoratorNames := make([]DecoratorName, 0, len(p.decorators))
	for name := range p.decorators {
		decoratorNames = append(decoratorNames, name)
	}

	return PlatformConfig{
		Name:           PlatformTypeOpenShift,
		ComponentNames: componentNames,
		DecoratorNames: decoratorNames,
	}
}

func (p *platformImpl) AllComponents() ComponentMap {
	return p.components
}

func (p *platformImpl) AllSupportedDecorators() DecoratorMap {
	return p.decorators
}
