package platform

import (
	"github.com/argoproj-labs/argocd-operator/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpenShift struct {
	controllers ControllerMap
}

func NewOpenShiftPlatform(client client.Client, scheme *runtime.Scheme) *OpenShift {
	OpenShift := &OpenShift{
		controllers: ControllerMap{
			"application-set": &component.ApplicationSetController{
				Client:                                client,
				Scheme:                                scheme,
				ManagedApplicationSetSourceNamespaces: nil,
			},
		},
	}
	return OpenShift
}

func (o *OpenShift) PlatformParams() PlatformConfig {
	//TODO implement me
	panic("implement me")
}

func (o *OpenShift) AllSupportedControllers() ControllerMap {
	return o.controllers
}

func (o *OpenShift) AllSupportedDecorators() DecoratorMap {
	//TODO implement me
	panic("implement me")
}
