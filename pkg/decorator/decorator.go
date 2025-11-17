package decorator

import (
	"github.com/go-logr/logr"
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

type SCCDecorator struct {
	Client client.Client
	Scheme *runtime.Scheme

	logger logr.Logger
}

func NewSCCDecorator(client client.Client, scheme *runtime.Scheme) *SCCDecorator {
	return &SCCDecorator{
		Client: client,
		Scheme: scheme,
		logger: logs.Log.WithName("ApplicationSetController"),
	}
}

func (s *SCCDecorator) Decorate(obj runtime.Object) {
	var podspec corev1.PodSpec
	switch obj.(type) {
	case *corev1.Pod:
		podspec = obj.(*corev1.Pod).Spec
	case *v1.Deployment:
		podspec = obj.(*v1.Deployment).Spec.Template.Spec
	case *v1.DaemonSet:
		podspec = obj.(*v1.DaemonSet).Spec.Template.Spec
	case *v1.ReplicaSet:
		podspec = obj.(*v1.ReplicaSet).Spec.Template.Spec
	case *v1.StatefulSet:
		podspec = obj.(*v1.StatefulSet).Spec.Template.Spec
	case *v2.Job:
		podspec = obj.(*v2.Job).Spec.Template.Spec
	case *v2.CronJob:
		podspec = obj.(*v2.CronJob).Spec.JobTemplate.Spec.Template.Spec
	}

	s.logger.Info("Decorating podspec for Security context constraints(SCC)", "podspec", podspec)
	if podspec.SecurityContext == nil {
		podspec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if podspec.SecurityContext.SeccompProfile == nil {
		podspec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{}
	}
	if len(podspec.SecurityContext.SeccompProfile.Type) == 0 {
		podspec.SecurityContext.SeccompProfile.Type = corev1.SeccompProfileTypeRuntimeDefault
	}
}
