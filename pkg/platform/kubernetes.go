package platform

import (
	"github.com/argoproj-labs/argocd-operator/pkg/component"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Kubernetes struct {
	controllers ControllerMap
}

func NewKubernetesPlatform(client client.Client, scheme *runtime.Scheme) *Kubernetes {
	Kubernetes := &Kubernetes{
		controllers: ControllerMap{
			"application-set": &component.ApplicationSetController{
				Client:                                client,
				Scheme:                                scheme,
				ManagedApplicationSetSourceNamespaces: nil,
			},
		},
	}
	return Kubernetes
}
func (k Kubernetes) PlatformParams() PlatformConfig {
	//TODO implement me
	panic("implement me")
}

func (k Kubernetes) AllSupportedControllers() ControllerMap {
	//TODO implement me
	panic("implement me")
}

func (k Kubernetes) AllSupportedDecorators() DecoratorMap {
	//TODO implement me
	panic("implement me")
}
