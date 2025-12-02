package decorator

import (
	"fmt"

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
		logger: logs.Log.WithName("SCCDecorator"),
	}
}

func (s *SCCDecorator) Decorate(obj runtime.Object) error {
	var podspec *corev1.PodSpec
	var objectName string

	switch typed := obj.(type) {
	case *corev1.Pod:
		podspec = &typed.Spec
		objectName = fmt.Sprintf("Pod/%s", typed.Name)
	case *v1.Deployment:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("Deployment/%s", typed.Name)
	case *v1.DaemonSet:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("DaemonSet/%s", typed.Name)
	case *v1.ReplicaSet:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("ReplicaSet/%s", typed.Name)
	case *v1.StatefulSet:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("StatefulSet/%s", typed.Name)
	case *v2.Job:
		podspec = &typed.Spec.Template.Spec
		objectName = fmt.Sprintf("Job/%s", typed.Name)
	case *v2.CronJob:
		podspec = &typed.Spec.JobTemplate.Spec.Template.Spec
		objectName = fmt.Sprintf("CronJob/%s", typed.Name)
	default:
		return fmt.Errorf("unsupported object type for SCC decoration: %T", obj)
	}

	s.logger.Info("Decorating podspec for Security Context Constraints (SCC)", "object", objectName)

	if podspec.SecurityContext == nil {
		podspec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if podspec.SecurityContext.SeccompProfile == nil {
		podspec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{}
	}
	if len(podspec.SecurityContext.SeccompProfile.Type) == 0 {
		podspec.SecurityContext.SeccompProfile.Type = corev1.SeccompProfileTypeRuntimeDefault
	}

	return nil
}
